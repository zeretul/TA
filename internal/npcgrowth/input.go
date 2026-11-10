package npcgrowth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const maxInputBytes = 1 << 20

// DecodeInput strictly decodes one complete JSON request, rejecting missing or
// null required fields, unknown fields, trailing values, and oversized input.
func DecodeInput(reader io.Reader) (Input, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxInputBytes+1))
	if err != nil {
		return Input{}, diagnostic("input_read_failed", "$", "Could not read the input.")
	}
	if len(data) > maxInputBytes {
		return Input{}, diagnostic("input_too_large", "$", "The input exceeds the size limit.")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Input{}, diagnostic("invalid_json", "$", "The input must be one JSON object.")
	}
	if err := validateRequiredShape(trimmed); err != nil {
		return Input{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var input Input
	if err := decoder.Decode(&input); err != nil {
		return Input{}, diagnostic("invalid_json", "$", "The input JSON is invalid or contains an unknown field.")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Input{}, diagnostic("trailing_json", "$", "The input must contain exactly one JSON value.")
	}
	return input, nil
}

func validateRequiredShape(data []byte) error {
	root, err := requireObject(data, "$", "schema_version", "actor_id", "actor_source_ref", "base_revision", "state", "interval")
	if err != nil {
		return err
	}
	state, err := requireObject(root["state"], "$.state", "source_ref", "revision", "cultivation", "accumulated", "settled_through_seconds", "last_interval_id", "last_interval_hash")
	if err != nil {
		return err
	}
	if _, err = requireObject(state["cultivation"], "$.state.cultivation", "realm", "stage", "layer"); err != nil {
		return err
	}
	if raw, exists := state["breakthrough"]; exists {
		breakthrough, err := requireObject(raw, "$.state.breakthrough", "current_health", "current_mana", "recovery_until_seconds")
		if err != nil {
			return err
		}
		if raw, exists := breakthrough["pending"]; exists {
			if err := requirePreparationShape(raw, "$.state.breakthrough.pending"); err != nil {
				return err
			}
		}
		if raw, exists := breakthrough["last_attempt"]; exists {
			if err := requireCompletedAttemptShape(raw, "$.state.breakthrough.last_attempt"); err != nil {
				return err
			}
		}
	}
	if raw, exists := root["breakthrough_intent"]; exists {
		intent, err := requireObject(raw, "$.breakthrough_intent", "attempt_id", "source_ref", "target", "technique_supports_target", "technique_source_ref", "applicability_source_ref")
		if err != nil {
			return err
		}
		if raw, exists := intent["random_receipt"]; exists {
			if _, err := requireObject(raw, "$.breakthrough_intent.random_receipt", "attempt_id", "source_ref", "roll", "condition_fingerprint"); err != nil {
				return err
			}
		}
	}
	interval, err := requireObject(root["interval"], "$.interval", "id", "source_ref", "from_seconds", "to_seconds", "segments")
	if err != nil {
		return err
	}
	var segments []json.RawMessage
	if err := json.Unmarshal(interval["segments"], &segments); err != nil || len(segments) == 0 {
		return diagnostic("invalid_segments", "$.interval.segments", "At least one segment is required.")
	}
	for index, raw := range segments {
		path := fmt.Sprintf("$.interval.segments[%d]", index)
		segment, err := requireObject(raw, path, "from_seconds", "to_seconds", "activity", "activity_source_ref", "opportunity_source_ref", "conditions")
		if err != nil {
			return err
		}
		if _, err = requireObject(segment["conditions"], path+".conditions", "source_ref", "talent", "technique_quality", "technique_source_ref", "growth_coefficient", "environment", "support_resource", "current_health", "maximum_health"); err != nil {
			return err
		}
	}
	return nil
}

func requirePreparationShape(data []byte, path string) error {
	fields, err := requireObject(data, path, "intent", "conditions", "started_at_seconds", "completes_at_seconds", "start_health", "start_mana", "mana_cost")
	if err != nil {
		return err
	}
	if err := requireIntentShape(fields["intent"], path+".intent", true); err != nil {
		return err
	}
	if _, err := requireObject(fields["conditions"], path+".conditions", conditionFieldNames...); err != nil {
		return err
	}
	return nil
}

func requireCompletedAttemptShape(data []byte, path string) error {
	fields, err := requireObject(data, path, "intent", "conditions", "started_at_seconds", "completed_at_seconds", "status", "condition_fingerprint")
	if err != nil {
		return err
	}
	if err := requireIntentShape(fields["intent"], path+".intent", true); err != nil {
		return err
	}
	if _, err := requireObject(fields["conditions"], path+".conditions", conditionFieldNames...); err != nil {
		return err
	}
	return nil
}

func requireIntentShape(data []byte, path string, receiptRequired bool) error {
	fields, err := requireObject(data, path, "attempt_id", "source_ref", "target", "technique_supports_target", "technique_source_ref", "applicability_source_ref")
	if err != nil {
		return err
	}
	raw, exists := fields["random_receipt"]
	if !exists {
		if receiptRequired {
			return diagnostic("required_field", path+".random_receipt", "A random receipt is required for a pending attempt.")
		}
		return nil
	}
	if _, err := requireObject(raw, path+".random_receipt", "attempt_id", "source_ref", "roll", "condition_fingerprint"); err != nil {
		return err
	}
	return nil
}

var conditionFieldNames = []string{
	"source_ref", "talent", "technique_quality", "technique_source_ref", "growth_coefficient",
	"environment", "support_resource", "current_health", "maximum_health",
}

func requireObject(data []byte, path string, required ...string) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, diagnostic("required_object", path, "A required object is missing or null.")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil || fields == nil {
		return nil, diagnostic("invalid_object", path, "The value must be a JSON object.")
	}
	for _, name := range required {
		value, exists := fields[name]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, diagnostic("required_field", path+"."+name, "A required field is missing or null.")
		}
	}
	return fields, nil
}

func diagnostic(code, path, message string) error {
	return &DiagnosticError{Diagnostics: []Diagnostic{{Code: code, Path: path, Message: message}}}
}
