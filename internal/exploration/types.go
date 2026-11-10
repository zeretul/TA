// Package exploration provides deterministic, storage-free route calculations,
// dungeon validation, authored exploration action proposals, and player-known
// projections. It never owns actor resources, Story clocks, or persistence.
package exploration

import (
	"fmt"
	"strings"
)

const (
	SchemaVersion       = 1
	MaxRouteSegments    = 4096
	MaxDefinitionBytes  = 4 << 20
	MaxIdentifierBytes  = 128
	MaxDescriptionBytes = 16 << 10
	MaxAreas            = 512
	MaxConnections      = 2048
	MaxObjectives       = 512
	MaxCompletionPaths  = 256
	MaxChallenges       = 1024
	MaxContentRefs      = 2048
	MaxKnowledgeItems   = 4096
	MaxQuantity         = 1_000_000_000
)

// ErrorCode is an immutable sentinel suitable for errors.Is.
type ErrorCode string

func (code ErrorCode) Error() string { return string(code) }

const (
	ErrInvalidRoute      ErrorCode = "invalid route"
	ErrInvalidProgress   ErrorCode = "invalid journey progress"
	ErrInvalidTarget     ErrorCode = "invalid elapsed-time target"
	ErrTimeOverflow      ErrorCode = "elapsed-time overflow"
	ErrInvalidDefinition ErrorCode = "invalid dungeon definition"
	ErrInvalidBinding    ErrorCode = "invalid dungeon instance binding"
	ErrInvalidKnowledge  ErrorCode = "invalid knowledge snapshot"
	ErrInputTooLarge     ErrorCode = "input exceeds supported size"
)

// Diagnostic points to one invalid field, reference, path, or projected fact.
// Paths and messages are deterministic English intended for tools and logs.
type Diagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func validID(value string) bool {
	if value == "" || len(value) > MaxIdentifierBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == ':' || r == '@' || r == '#' {
			continue
		}
		return false
	}
	return true
}

func validText(value string, maxBytes int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= maxBytes
}

func requireID(value, path string) []Diagnostic {
	if validID(value) {
		return nil
	}
	return []Diagnostic{{Code: "invalid_id", Path: path, Message: "must be a stable non-empty identifier"}}
}

func requireText(value, path string, maxBytes int) []Diagnostic {
	if validText(value, maxBytes) {
		return nil
	}
	return []Diagnostic{{Code: "invalid_text", Path: path, Message: fmt.Sprintf("must be non-empty and at most %d UTF-8 bytes", maxBytes)}}
}

func appendDiagnostics(target *[]Diagnostic, additions ...Diagnostic) {
	*target = append(*target, additions...)
}
