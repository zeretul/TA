package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"ta/internal/storytime"
)

type request struct {
	CurrentSeconds *int64         `json:"current_seconds"`
	Activity       *activityInput `json:"activity"`
	Nodes          *[]nodeInput   `json:"nodes"`
}

type activityInput struct {
	ID                 *string   `json:"id"`
	StoryID            *string   `json:"story_id"`
	BranchID           *string   `json:"branch_id"`
	SourceID           *string   `json:"source_id"`
	Mode               *string   `json:"mode"`
	Status             *string   `json:"status"`
	StartedAt          *int64    `json:"started_at_seconds"`
	LastSettledAt      *int64    `json:"last_settled_at_seconds"`
	FixedEndAt         *int64    `json:"fixed_end_at_seconds"`
	RequiredEffective  *int64    `json:"required_effective_seconds"`
	CompletedEffective *int64    `json:"completed_effective_seconds"`
	PendingNodeIDs     *[]string `json:"pending_node_ids"`
}

type nodeInput struct {
	ID                *string `json:"id"`
	At                *int64  `json:"at_seconds"`
	Kind              *string `json:"kind"`
	RequiresResponse  *bool   `json:"requires_response"`
	Resolved          *bool   `json:"resolved"`
	PublicDescription *string `json:"public_description"`
}

type diagnostic struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func main() {
	inputPath := flag.String("input", "", "read a synthetic snapshot JSON file; stdin is used when omitted")
	flag.Parse()
	if flag.NArg() != 0 {
		fail(fmt.Errorf("unexpected positional arguments"))
	}
	var input io.Reader = os.Stdin
	if *inputPath != "" {
		file, err := os.Open(*inputPath)
		if err != nil {
			fail(fmt.Errorf("open input: %w", err))
		}
		defer file.Close()
		input = file
	}
	data, err := io.ReadAll(io.LimitReader(input, (1<<20)+1))
	if err != nil {
		fail(fmt.Errorf("read input: %w", err))
	}
	if len(data) > 1<<20 {
		fail(fmt.Errorf("input exceeds the 1 MiB limit"))
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var raw request
	if err := decoder.Decode(&raw); err != nil {
		fail(fmt.Errorf("decode input: %w", err))
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			fail(fmt.Errorf("input contains more than one JSON value"))
		}
		fail(fmt.Errorf("read input end: %w", err))
	}
	plan, err := evaluate(raw)
	if err != nil {
		fail(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(plan); err != nil {
		fail(fmt.Errorf("encode result: %w", err))
	}
}

func evaluate(raw request) (storytime.Plan, error) {
	if raw.CurrentSeconds == nil || raw.Activity == nil || raw.Nodes == nil {
		return storytime.Plan{}, fmt.Errorf("current_seconds, activity, and nodes are required")
	}
	ai := raw.Activity
	if ai.ID == nil || ai.StoryID == nil || ai.BranchID == nil || ai.SourceID == nil || ai.Mode == nil || ai.Status == nil || ai.StartedAt == nil || ai.LastSettledAt == nil || ai.CompletedEffective == nil {
		return storytime.Plan{}, fmt.Errorf("activity identity, mode, status, start, last-settled time, and completed-effective time are required")
	}
	activity := storytime.Activity{ID: *ai.ID, StoryID: *ai.StoryID, BranchID: *ai.BranchID, SourceID: *ai.SourceID,
		Mode: storytime.ActivityMode(*ai.Mode), Status: storytime.ActivityStatus(*ai.Status), StartedAt: storytime.Instant(*ai.StartedAt),
		LastSettledAt: storytime.Instant(*ai.LastSettledAt), CompletedEffective: storytime.Duration(*ai.CompletedEffective)}
	if ai.PendingNodeIDs != nil {
		activity.PendingNodeIDs = append([]string(nil), (*ai.PendingNodeIDs)...)
	}
	switch activity.Mode {
	case storytime.FixedEndpoint:
		if ai.FixedEndAt == nil || ai.RequiredEffective != nil {
			return storytime.Plan{}, fmt.Errorf("fixed_endpoint requires fixed_end_at_seconds and forbids required_effective_seconds")
		}
		activity.FixedEndAt = storytime.Instant(*ai.FixedEndAt)
	case storytime.EffectiveDuration:
		if ai.RequiredEffective == nil || ai.FixedEndAt != nil {
			return storytime.Plan{}, fmt.Errorf("effective_duration requires required_effective_seconds and forbids fixed_end_at_seconds")
		}
		activity.RequiredEffective = storytime.Duration(*ai.RequiredEffective)
	default:
		return storytime.Plan{}, fmt.Errorf("unsupported activity mode %q", activity.Mode)
	}
	nodes := make([]storytime.Node, 0, len(*raw.Nodes))
	for index, n := range *raw.Nodes {
		if n.ID == nil || n.At == nil || n.Kind == nil || n.RequiresResponse == nil || n.Resolved == nil {
			return storytime.Plan{}, fmt.Errorf("nodes[%d] requires id, at_seconds, kind, requires_response, and resolved", index)
		}
		node := storytime.Node{ID: *n.ID, At: storytime.Instant(*n.At), Kind: storytime.NodeKind(*n.Kind), RequiresResponse: *n.RequiresResponse, Resolved: *n.Resolved}
		if n.PublicDescription != nil {
			node.PublicDescription = *n.PublicDescription
		}
		nodes = append(nodes, node)
	}
	return storytime.PlanActivity(activity, storytime.Instant(*raw.CurrentSeconds), nodes)
}

func fail(err error) {
	var d diagnostic
	d.Error.Code = "invalid_input"
	d.Error.Message = err.Error()
	_ = json.NewEncoder(os.Stderr).Encode(d)
	os.Exit(2)
}
