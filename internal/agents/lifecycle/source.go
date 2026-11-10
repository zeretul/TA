package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agentchat "ta/internal/agents/chat"
	agentrun "ta/internal/agents/run"

	agent "ta/agent"
)

const turnHostDataType = "denova.turn"

// TurnKind is Text Adventure's product delivery semantic. It is persisted in HostData
// because a recovered public Agent cycle must not infer FollowUp versus
// NextTurn from process-local control flow or from the cycle ordinal.
type TurnKind string

const (
	TurnStart    TurnKind = "start"
	TurnSteer    TurnKind = "steer"
	TurnFollowUp TurnKind = "follow_up"
	TurnNext     TurnKind = "next_turn"
)

// TurnHostData is the bounded, versioned product input needed to rebuild one
// cycle after admission. Mutable workspace content and resolved canonical text
// are deliberately absent and must be loaded by the product resolver.
type TurnHostData struct {
	Kind            TurnKind                 `json:"kind"`
	Caller          agentchat.CallerInput    `json:"caller"`
	InputVisibility agentrun.InputVisibility `json:"input_visibility,omitempty"`
	AutomationID    string                   `json:"automation_id,omitempty"`
	ReviewThreadID  string                   `json:"review_thread_id,omitempty"`
	TurnID          string                   `json:"turn_id,omitempty"`
	MaintenanceTask string                   `json:"maintenance_task,omitempty"`
	Mode            string                   `json:"mode,omitempty"`
	WriteMode       string                   `json:"write_mode,omitempty"`
	WriteScope      string                   `json:"write_scope,omitempty"`
	RestoreData     *agentrun.RestoreData    `json:"restore_data,omitempty"`
}

// TurnInput converts Text Adventure transport semantics into the public Agent Input.
// The user text remains first-class while product-only fields stay in HostData.
func TurnInput(kind TurnKind, request agentchat.ChatRequest, options agentrun.Options) (agent.Input, error) {
	switch kind {
	case TurnStart, TurnSteer, TurnFollowUp, TurnNext:
	default:
		return agent.Input{}, fmt.Errorf("Text Adventure Agent turn kind %q is invalid", kind)
	}
	request = agentchat.CaptureChatRequestCallerInput(request)
	caller := agentchat.CallerView(request)
	if strings.TrimSpace(caller.Message) == "" && len(request.AttachedFiles) == 0 {
		return agent.Input{}, errors.New("Text Adventure Agent turn requires a message or attachments")
	}
	if options.RestoreData != nil && (strings.TrimSpace(options.RestoreData.Type) == "" ||
		options.RestoreData.Version == 0 || !json.Valid(options.RestoreData.Data)) {
		return agent.Input{}, errors.New("Text Adventure Agent turn product restore data is invalid")
	}
	data := TurnHostData{
		Kind: kind, Caller: caller, InputVisibility: request.InputVisibility,
		AutomationID:   strings.TrimSpace(options.AutomationTaskID),
		ReviewThreadID: strings.TrimSpace(options.ReviewThreadID), TurnID: strings.TrimSpace(options.TurnID),
		MaintenanceTask: strings.TrimSpace(options.MaintenanceTask), Mode: strings.TrimSpace(options.Mode),
		WriteMode: strings.TrimSpace(options.WriteMode), WriteScope: strings.TrimSpace(options.WriteScope),
		RestoreData: options.RestoreData,
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return agent.Input{}, fmt.Errorf("encode Text Adventure Agent turn HostData: %w", err)
	}
	return agent.Input{
		Text: caller.Message, Attachments: append([]agent.Attachment(nil), request.AttachedFiles...),
		IdempotencyKey: strings.TrimSpace(caller.CommandID),
		HostData:       &agent.HostData{Type: turnHostDataType, Version: 2, Data: encoded},
	}, nil
}

func DecodeTurnHostData(input agent.Input) (TurnHostData, error) {
	return decodeTurnHostData(input.HostData, input.Text)
}

// DecodeTurnHostDataFromPrepare also supports canonical/effect recovery, where
// Agent supplies only the durable HostData descriptor.
func DecodeTurnHostDataFromPrepare(request agent.PrepareRequest) (TurnHostData, error) {
	hostData := request.Input.HostData
	if hostData == nil {
		hostData = request.HostData
	}
	return decodeTurnHostData(hostData, request.Input.Text)
}

func decodeTurnHostData(hostData *agent.HostData, inputText string) (TurnHostData, error) {
	if hostData == nil || hostData.Type != turnHostDataType || hostData.Version != 2 {
		return TurnHostData{}, errors.New("Text Adventure Agent turn HostData is unavailable")
	}
	var data TurnHostData
	if err := json.Unmarshal(hostData.Data, &data); err != nil {
		return TurnHostData{}, fmt.Errorf("decode Text Adventure Agent turn HostData: %w", err)
	}
	switch data.Kind {
	case TurnStart, TurnSteer, TurnFollowUp, TurnNext:
	default:
		return TurnHostData{}, errors.New("Text Adventure Agent turn HostData has an invalid turn kind")
	}
	if strings.TrimSpace(data.Caller.Message) == "" && len(data.Caller.AttachmentIDs) == 0 ||
		inputText != "" && data.Caller.Message != inputText {
		return TurnHostData{}, errors.New("Text Adventure Agent turn HostData does not match public Input")
	}
	if data.RestoreData != nil {
		if strings.TrimSpace(data.RestoreData.Type) == "" || data.RestoreData.Version == 0 || !json.Valid(data.RestoreData.Data) {
			return TurnHostData{}, errors.New("Text Adventure Agent turn HostData has invalid product restore data")
		}
		cloned := *data.RestoreData
		cloned.Type = strings.TrimSpace(cloned.Type)
		cloned.Data = append(json.RawMessage(nil), cloned.Data...)
		data.RestoreData = &cloned
	}
	return data, nil
}

func (data TurnHostData) ChatRequest() agentchat.ChatRequest {
	return agentchat.ChatRequest{
		CommandID: data.Caller.CommandID, Message: data.Caller.Message, DisplayMessage: data.Caller.DisplayMessage,
		AttachmentIDs: append([]string(nil), data.Caller.AttachmentIDs...),
		References:    append([]string(nil), data.Caller.References...), LoreReferences: append([]string(nil), data.Caller.LoreReferences...),
		StyleScenes: append([]string(nil), data.Caller.StyleScenes...), Selections: append([]agentchat.TextSelectionRef(nil), data.Caller.Selections...),
		IDEContext: data.Caller.IDEContext, ReviewFeedback: data.Caller.ReviewFeedback.Clone(),
		ImagePresetID: data.Caller.ImagePresetID, TellerID: data.Caller.TellerID,
		Locale: data.Caller.Locale, InputVisibility: data.InputVisibility,
	}
}

// DefinitionRequest is Text Adventure's product composition seam. It contains product
// identity and the public lifecycle request, but no runtime implementation.
type DefinitionRequest struct {
	Binding agentrun.RuntimeBinding
	Agent   agent.PrepareRequest
}

type DefinitionResolver interface {
	ResolveDefinition(context.Context, DefinitionRequest) (agent.Definition, error)
}

// CanonicalInputResolver is the narrow admission counterpart to
// DefinitionResolver. It must not prepare a model, Toolset, or Context; the
// public Runtime calls it before Definition preparation can be preempted.
type CanonicalInputResolver interface {
	ResolveCanonicalInput(context.Context, DefinitionRequest) (agent.CanonicalAdapter, error)
}

type DefinitionResolverFunc func(context.Context, DefinitionRequest) (agent.Definition, error)

func (resolve DefinitionResolverFunc) ResolveDefinition(ctx context.Context, request DefinitionRequest) (agent.Definition, error) {
	if resolve == nil {
		return agent.Definition{}, errors.New("Text Adventure Agent Definition resolver is nil")
	}
	return resolve(ctx, request)
}

type taSource struct{ resolver DefinitionResolver }

// NewSource adapts Text Adventure Session identity to the public Agent Source seam.
func NewSource(resolver DefinitionResolver) (agent.Source, error) {
	if resolver == nil {
		return nil, errors.New("Text Adventure Agent Definition resolver is required")
	}
	return taSource{resolver: resolver}, nil
}

func (source taSource) Prepare(ctx context.Context, request agent.PrepareRequest) (agent.Definition, error) {
	return source.resolve(ctx, request)
}

func (source taSource) CanonicalInput(ctx context.Context, request agent.PrepareRequest) (agent.CanonicalAdapter, error) {
	if strings.HasPrefix(request.Session.Key.Namespace, "task.") || request.Reason == agent.TurnReasonGoalMutation {
		return nil, nil
	}
	resolver, ok := source.resolver.(CanonicalInputResolver)
	if !ok {
		return nil, errors.New("Text Adventure Agent Definition resolver has no provider-free canonical input boundary")
	}
	binding, err := agentrun.RuntimeBindingFromAgentSessionKey(request.Session.Key)
	if err != nil {
		return nil, fmt.Errorf("resolve Text Adventure Agent Session binding: %w", err)
	}
	return resolver.ResolveCanonicalInput(ctx, DefinitionRequest{Binding: binding, Agent: request})
}

func (source taSource) resolve(ctx context.Context, request agent.PrepareRequest) (agent.Definition, error) {
	binding, err := agentrun.RuntimeBindingFromAgentSessionKey(request.Session.Key)
	if err != nil {
		// Delegated Sessions intentionally use the public task namespace. Their
		// resolver reconstructs the parent product binding from immutable child
		// key attributes and therefore receives a zero Binding here.
		if !strings.HasPrefix(request.Session.Key.Namespace, "task.") {
			return agent.Definition{}, fmt.Errorf("resolve Text Adventure Agent Session binding: %w", err)
		}
		return source.resolver.ResolveDefinition(ctx, DefinitionRequest{Agent: request})
	}
	return source.resolver.ResolveDefinition(ctx, DefinitionRequest{Binding: binding, Agent: request})
}
