package configresource

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agent "ta/agent"

	"ta/config"
)

const defaultToolResultMaxBytes = config.DefaultAgentToolResultLimitKB * 1024

type configReadInput struct {
	Operation string   `json:"operation" jsonschema:"enum=describe,enum=list,enum=get" jsonschema_description:"describe returns resource contracts; list returns a cursor-paginated catalog; get reads exact IDs with partial-success reporting."`
	Resource  string   `json:"resource,omitempty" jsonschema_description:"Resource kind. Omit only when operation=describe to inspect every available resource."`
	IDs       []string `json:"ids,omitempty" jsonschema_description:"Exact resource IDs for operation=get. There is no arbitrary item-count limit; large responses continue with next_cursor."`
	Scope     string   `json:"scope,omitempty" jsonschema_description:"Optional resource scope, such as user or workspace."`
	Query     string   `json:"query,omitempty" jsonschema_description:"Optional adapter-specific catalog filter."`
	Cursor    string   `json:"cursor,omitempty" jsonschema_description:"Opaque continuation cursor returned by an earlier identical list or get request."`
	Limit     int      `json:"limit,omitempty" jsonschema:"minimum=1" jsonschema_description:"Optional page size. The shared tool-result byte budget may return fewer items with next_cursor."`
}

type configApplyInput struct {
	Operation string         `json:"operation" jsonschema:"enum=create,enum=update,enum=delete" jsonschema_description:"One independently committed mutation."`
	Resource  string         `json:"resource" jsonschema_description:"Resource kind returned by config_read describe."`
	ID        string         `json:"id,omitempty" jsonschema_description:"Exact resource ID; required for update and delete."`
	Scope     string         `json:"scope,omitempty" jsonschema_description:"Required for scoped resources such as skill and agent_profile."`
	Revision  string         `json:"revision,omitempty" jsonschema_description:"Required for update and delete, and for agent_profile SubAgent create; custom Agent create has the same exact-scope revision requirement. Copy it from config_read."`
	Value     map[string]any `json:"value,omitempty" jsonschema_description:"Complete create/update value documented by the resource Skill reference; agent_profile delete requires value.kind."`
}

// ResourceNames lists every configuration resource the shared registry serves.
// Callers that must narrow the surface (the Game Settings assistant) name the
// subset they keep; the general Writing and General Agents keep all of them.
var ResourceNames = []string{
	"style_reference", "narrative_style", "game_planning", "event_package",
	"rule_system", "state_system", "image_preset", "skill", "agent_profile",
}

// NewTools constructs the shared configuration tools over the typed resource
// registry. Capability filtering remains with the caller that owns the full
// model-visible tool catalog.
func NewTools(cfg *config.Config, maxResultBytes int) ([]agent.ToolDefinition, error) {
	return NewToolsWithResources(cfg, maxResultBytes, nil)
}

// NewToolsWithResources restricts the registry to exactly the named resources.
// A nil or empty list keeps every resource, so existing callers are unchanged.
//
// This is a real narrowing, not a prompt instruction: an omitted adapter has no
// entry in the registry, so config_read cannot describe it and config_apply
// fails with an unknown-resource error before any store is opened.
func NewToolsWithResources(cfg *config.Config, maxResultBytes int, resources []string) ([]agent.ToolDefinition, error) {
	return NewScopedTools(cfg, maxResultBytes, resources, nil)
}

// NewScopedTools narrows the registry further to a bound target: the shared
// preset libraries become read-only templates, and the two settings resources
// this conversation may write are its own story and its own world.
//
// A nil target keeps the previous behaviour exactly, so the general Writing and
// General Agents still edit presets in place. A non-nil target with no
// identifiers is still a narrowing: it has no settings resource to write and
// every shared library is read-only, which is the refusal to fall back to the
// shared write scope rather than a permission to use it.
func NewScopedTools(cfg *config.Config, maxResultBytes int, resources []string, target *ScopedTarget) ([]agent.ToolDefinition, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	registry, err := newConfigResourceRegistry(cfg, resources, target)
	if err != nil {
		return nil, err
	}
	if maxResultBytes <= 0 {
		maxResultBytes = defaultToolResultMaxBytes
	}
	readTool, err := agent.InferTool(
		"config_read",
		"Inspect, list, or read Text Adventure configuration resources through one typed registry. List and large exact reads return stable continuation cursors. Exact reads return existing items plus missing_ids/failures; only an entirely unsuccessful completed request fails. Call operation=describe before using an unfamiliar resource and keep returned revisions for later mutations.",
		func(ctx context.Context, input configReadInput) (string, error) {
			value, err := registry.Read(ctx, ReadRequest{
				Operation: input.Operation, Resource: input.Resource, IDs: input.IDs, Scope: input.Scope, Query: input.Query,
				Cursor: input.Cursor, Limit: input.Limit, ResultMaxBytes: maxResultBytes,
			})
			if err != nil {
				return "", err
			}
			return marshalConfigResourceResult(value)
		},
	)
	if err != nil {
		return nil, err
	}
	applyTool, err := agent.InferTool(
		"config_apply",
		"Create, update, or delete exactly one Text Adventure configuration resource. Updates, deletes, and agent_profile SubAgent creates require the latest revision from config_read; custom Agent creates use the same requirement. agent_profile deletes require value.kind. Resource-specific value shapes live in the configuration Skill references.",
		func(ctx context.Context, input configApplyInput) (agent.ToolResult, error) {
			value, err := registry.Apply(ctx, Mutation{
				Operation: input.Operation, Resource: input.Resource, ID: input.ID, Scope: input.Scope, Revision: input.Revision, Value: input.Value,
			})
			if err != nil {
				return agent.ToolResult{}, err
			}
			return configApplyResult(value)
		},
	)
	if err != nil {
		return nil, err
	}
	readDefinition, err := defineTool(readTool, configReadDescriptor(maxResultBytes))
	if err != nil {
		return nil, err
	}
	applyDefinition, err := defineTool(applyTool, configApplyDescriptor(maxResultBytes))
	if err != nil {
		return nil, err
	}
	return []agent.ToolDefinition{readDefinition, applyDefinition}, nil
}

func defineTool(tool agent.Tool, descriptor agent.ToolDescriptor) (agent.ToolDefinition, error) {
	definition := agent.ToolDefinition{Tool: tool, Descriptor: descriptor}
	if err := definition.Validate(context.Background()); err != nil {
		return agent.ToolDefinition{}, err
	}
	return definition, nil
}

func newConfigResourceRegistry(cfg *config.Config, resources []string, target *ScopedTarget) (*Registry, error) {
	novaDir := strings.TrimSpace(cfg.DataDir())
	// A purpose-scoped conversation must not be able to overwrite a shared
	// preset in place: those libraries are process-wide, so an edit through one
	// world's assistant would change every world. They stay as read sources and
	// the writable surface is the two bound settings resources instead.
	shared := func(adapter Adapter) Adapter {
		if target == nil {
			return adapter
		}
		return readOnlyTemplate(adapter)
	}
	all := map[string]Adapter{
		"style_reference": shared(newStyleReferenceResource(novaDir)),
		"narrative_style": shared(newNarrativeStyleResource(novaDir)),
		"game_planning":   shared(newGamePlanningResource(novaDir)),
		"event_package":   shared(newEventPackageResource(novaDir)),
		"rule_system":     shared(newRuleSystemResource(novaDir)),
		"state_system":    shared(newStateSystemResource(novaDir)),
		"image_preset":    shared(newImagePresetResource(novaDir)),
		"skill":           newSkillConfigResource(cfg),
		"agent_profile":   newAgentProfileResource(cfg),
	}
	if target != nil {
		all[StorySettingsResource] = newStorySettingsResource(target)
		all[WorldDefaultsResource] = newWorldDefaultsResource(target)
		all[WorldBookResource] = newWorldBookResource(target)
	}
	if len(resources) == 0 {
		resources = append([]string(nil), ResourceNames...)
	}
	// The bound settings resources are the only writable configuration a scoped
	// conversation has, so they are always part of its surface. They are added
	// here rather than by every caller so a caller cannot name a resource set
	// that leaves the assistant with nothing it may write.
	if target != nil {
		resources = append(resources, StorySettingsResource, WorldDefaultsResource, WorldBookResource)
	}
	selected := make([]Adapter, 0, len(resources))
	seen := make(map[string]struct{}, len(resources))
	for _, name := range resources {
		name = strings.TrimSpace(name)
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		adapter, ok := all[name]
		if !ok {
			return nil, fmt.Errorf("unknown configuration resource %q", name)
		}
		seen[name] = struct{}{}
		selected = append(selected, adapter)
	}
	return New(selected...)
}

func configReadDescriptor(maxResultBytes ...int) agent.ToolDescriptor {
	limit := defaultToolResultMaxBytes
	if len(maxResultBytes) > 0 && maxResultBytes[0] > 0 {
		limit = maxResultBytes[0]
	}
	return agent.ToolDescriptor{
		Source: agent.ToolSourceRead, Capability: config.AgentToolConfigRead, Execution: agent.ToolExecutionParallelRead,
		MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone, Recovery: agent.ToolRecoveryReadOnly,
		ResultRecoveryKind: agent.ToolResultRecoveryRerun,
		ResultProjection:   agent.ToolResultBoundedModelContext, ResultRetention: agent.ToolResultDeferred,
		Steering: agent.SteeringFinishCurrent, MaxResultBytes: limit,
	}
}

func configApplyDescriptor(maxResultBytes ...int) agent.ToolDescriptor {
	limit := defaultToolResultMaxBytes
	if len(maxResultBytes) > 0 && maxResultBytes[0] > 0 {
		limit = maxResultBytes[0]
	}
	return agent.ToolDescriptor{
		Source: agent.ToolSourceWrite, Capability: config.AgentToolConfigApply, Execution: agent.ToolExecutionConfigExclusive,
		MutationScope: agent.ToolMutationConfig, PostCheck: agent.ToolPostCheckConfigRevision, Recovery: agent.ToolRecoveryReconcilable,
		ResultProjection: agent.ToolResultBoundedModelContext, ResultRetention: agent.ToolResultProtected,
		Steering: agent.SteeringFinishCurrent, MaxResultBytes: limit,
	}
}

func marshalConfigResourceResult(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode config resource result: %w", err)
	}
	return string(data), nil
}

type configApplyReceiptDetails struct {
	Schema    string `json:"schema"`
	Status    string `json:"status"`
	Resource  string `json:"resource"`
	Operation string `json:"operation"`
	ID        string `json:"id"`
	Revision  string `json:"revision,omitempty"`
}

func configApplyResult(value any) (agent.ToolResult, error) {
	receipt, ok := value.(configMutationReceipt)
	if !ok {
		return agent.ToolResult{}, fmt.Errorf("config resource Adapter returned %T, want configMutationReceipt", value)
	}
	details, err := json.Marshal(configApplyReceiptDetails{
		Schema: "config.mutation_receipt.v1", Status: "applied",
		Resource: receipt.Resource, Operation: receipt.Operation,
		ID: receipt.ID, Revision: receipt.Revision,
	})
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("encode config mutation receipt: %w", err)
	}
	// Mutation output stays receipt-only so a large, successfully persisted
	// configuration cannot be followed by an invalid or truncated JSON echo.
	// The configuration workflow always verifies the canonical value with get.
	result := agent.TextToolResult(string(details))
	result.Details = details
	result.Metadata.Target = receipt.ID
	return result, nil
}

func firstConfigNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
