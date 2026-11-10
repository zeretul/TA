package history

import (
	"fmt"
	"strings"
)

func orderActive(status string) bool { return status == "active" || status == "waiting" }

func validateOrderIntent(s State, in OrderIntent) error {
	if in.Disclosure != nil && *in.Disclosure != "public" && *in.Disclosure != "sealed" {
		return invalid("invalid order disclosure")
	}
	if s.World == nil {
		return invalid("world required")
	}
	if in.Action != "create" {
		o := orderByID(s.World, in.OrderID)
		if o == nil {
			return invalid("order not found")
		}
		if o.Status == "completed" || o.Status == "revoked" {
			return invalid("order is closed")
		}
		switch in.Action {
		case "revise", "pause", "resume", "revoke":
		default:
			return invalid("unknown order action")
		}
		if in.Action != "revise" {
			return nil
		}
	}
	if in.Kind != "one_shot" && in.Kind != "project" && in.Kind != "policy" && in.Kind != "conditional" {
		return invalid("unknown order kind")
	}
	if strings.TrimSpace(in.Text) == "" || len(in.Text) > 12000 || len(in.Condition) > 4000 || len(in.Targets) > 32 || len(in.DependsOn) > 32 || in.NotBeforeTurn < 0 {
		return invalid("invalid order content")
	}
	if in.Kind == "conditional" && strings.TrimSpace(in.Condition) == "" {
		return invalid("conditional order requires a condition")
	}
	if in.MinisterID != "" && minister(s, in.MinisterID) == nil {
		return invalid("minister not available")
	}
	for _, id := range in.Targets {
		if entityByID(s.World, id) == nil {
			return invalid("order target %q is not an existing entity; use a known parent region or entity and keep new place names in text", id)
		}
	}
	for _, id := range in.DependsOn {
		if id == in.OrderID || orderByID(s.World, id) == nil {
			return invalid("order dependency not found")
		}
	}
	for id, v := range in.Limits {
		if resourceDef(*s.Scenario, id) == nil || v < 0 || v > 1000000000000 {
			return invalid("invalid spending limit")
		}
		if in.Action == "revise" && v < orderByID(s.World, in.OrderID).Spent[id] {
			return invalid("new cap is below actual expenditure")
		}
	}
	return nil
}

func validateWorldOrders(s State, plan EdictPlan) []Outcome {
	out := []Outcome{}
	seen := map[string]bool{}
	for i, in := range plan.Orders {
		o := Outcome{InstructionID: fmt.Sprintf("order-%d", i+1), ProjectID: in.OrderID, Status: "ready", Reason: "ready"}
		err := validateOrderIntent(s, in)
		if in.OrderID != "" && seen[in.OrderID] {
			err = invalid("conflicting amendments in one decree")
		}
		seen[in.OrderID] = true
		if err != nil {
			o.Status = "unsupported"
			o.Reason = err.Error()
		}
		out = append(out, o)
	}
	return out
}

func applyWorldEdict(s *State, id string, plan EdictPlan) ([]Outcome, error) {
	if s.World.Ending != "" {
		return nil, invalid("campaign has ended")
	}
	if len(plan.Orders) < 1 || len(plan.Orders) > 32 || len(plan.Instructions) > 0 {
		return nil, invalid("world decree requires 1 to 32 orders")
	}
	out := validateWorldOrders(*s, plan)
	// A decree is the player's complete intention: never silently execute a
	// supported subset while discarding its unrepresentable conditions.
	for _, o := range out {
		if o.Status != "ready" {
			return nil, invalid("decree requires correction: %s", o.Reason)
		}
	}
	for i, in := range plan.Orders {
		if in.Action == "create" {
			oid := shortID("order-", fmt.Sprintf("%s:%d", id, i))
			disclosure := "public"
			if in.Disclosure != nil {
				disclosure = *in.Disclosure
			}
			s.World.Orders = append(s.World.Orders, Order{Disclosure: disclosure, ID: oid, Version: 1, EdictID: id, Kind: in.Kind, Text: in.Text, MinisterID: in.MinisterID, Targets: in.Targets, Condition: in.Condition, DependsOn: in.DependsOn, NotBeforeTurn: in.NotBeforeTurn, Limits: in.Limits, Spent: map[string]int64{}, Status: "active", CreatedTurn: s.Turn})
			out[i].ProjectID = oid
		} else {
			o := orderByID(s.World, in.OrderID)
			o.Version++
			switch in.Action {
			case "pause":
				o.Status = "paused"
			case "resume":
				o.Status = "active"
			case "revoke":
				o.Status = "revoked"
			case "revise":
				if in.Disclosure != nil {
					o.Disclosure = *in.Disclosure
				}
				o.Kind = in.Kind
				o.Text = in.Text
				o.MinisterID = in.MinisterID
				o.Targets = in.Targets
				o.Condition = in.Condition
				o.DependsOn = in.DependsOn
				o.NotBeforeTurn = in.NotBeforeTurn
				o.Limits = in.Limits
				o.Status = "active"
			}
		}
		out[i].Status = "accepted"
		out[i].Reason = "edict_registered"
	}
	// References to existing orders can form a cycle after revision.
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if done[id] {
			return false
		}
		visiting[id] = true
		o := orderByID(s.World, id)
		if o != nil {
			for _, dep := range o.DependsOn {
				if visit(dep) {
					return true
				}
			}
		}
		visiting[id] = false
		done[id] = true
		return false
	}
	for _, o := range s.World.Orders {
		if visit(o.ID) {
			return nil, invalid("order dependency cycle")
		}
	}
	s.Edicts = append(s.Edicts, Edict{ID: id, Turn: s.Turn, Plan: plan, Outcomes: out})
	return out, nil
}

func orderReady(w *World, o Order, turn int) bool {
	if !orderActive(o.Status) || o.NotBeforeTurn > turn {
		return false
	}
	for _, dep := range o.DependsOn {
		p := orderByID(w, dep)
		if p == nil || p.Status != "completed" {
			return false
		}
	}
	return true
}
