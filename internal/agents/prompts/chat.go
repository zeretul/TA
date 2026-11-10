package prompts

import (
	"fmt"
	"strings"
)

// ContextBoundary precedes a user message and distinguishes the current request
// from workspace state and conversation history that provide background only.
func ContextBoundary(message string) string {
	return `[Context Boundary]
- The current user request defines what to do now. Act only on this turn's request, explicit @ references, # scene-style selection, and editor selection.
- Workspace state and confirmed novel state provide background only; they do not replace the explicit current request.
- Conversation history may help interpret context, but prior todos, tool intentions, or unfinished actions are not current instructions unless the user explicitly continues them in this turn.
- If the current request is unrelated to or conflicts with history, follow the current request and do not continue prior tool calls or modifications.
- If this turn explicitly requires a tool, actually call it in this turn and answer from the current tool result. Without a current result, do not claim to have called, read, searched, verified, or modified anything.

Current request:
` + message
}

// InterruptedResume describes the state left by an interrupted previous turn.
type InterruptedResume struct {
	UserMessage      string
	AssistantContent string
	Reason           string
}

// ResumeFromInterruption adds the previous interruption state when the user asks to continue.
func ResumeFromInterruption(current string, prev InterruptedResume) string {
	var sb strings.Builder
	sb.WriteString("[Interrupted Run Recovery]\n")
	sb.WriteString("The user asked to continue. Resume from the previous interruption without redoing work that was already completed and written to files.\n")
	sb.WriteString("Treat any partial assistant output from the previous run as completed context and continue the original request.\n\n")
	sb.WriteString("Previous original request:\n")
	sb.WriteString(prev.UserMessage)
	if prev.AssistantContent != "" {
		sb.WriteString("\n\nAssistant content generated before the interruption:\n")
		sb.WriteString(prev.AssistantContent)
	}
	if prev.Reason != "" {
		sb.WriteString("\n\nInterruption reason:\n")
		sb.WriteString(prev.Reason)
	}
	sb.WriteString("\n\nCurrent continuation request:\n")
	sb.WriteString(current)
	return sb.String()
}

// StyleReference identifies a shared style reference the Agent can read by path.
type StyleReference struct {
	Name        string
	Description string
	Path        string
	DisplayPath string
	Missing     bool
	Error       string
}

// StyleRule maps either the global scope or a scene to a style reference.
type StyleRule struct {
	Global          bool
	Scene           string
	StyleReferences []StyleReference
	StyleContents   []string
}

// StyleRulesInstruction renders the director's style index as a stable system prompt fragment.
func StyleRulesInstruction(rules []StyleRule) string {
	var sb strings.Builder
	sb.WriteString("## Prose Style References\n\n")
	sb.WriteString("The current narrative style provides the following prose-style reference index. References are shared Markdown files stored under `.ta-game/styles/`; the index provides only name, description, and path, not full content.\n")
	wrote := 0
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if !rule.Global && scene == "" {
			continue
		}
		if len(rule.StyleReferences) == 0 && len(rule.StyleContents) == 0 {
			continue
		}
		wrote++
		if rule.Global {
			fmt.Fprintf(&sb, "%d. Global prose-style references: apply to all prose generation by default\n", wrote)
		} else {
			fmt.Fprintf(&sb, "%d. Scene: %s\n", wrote, scene)
		}
		for _, ref := range rule.StyleReferences {
			name := strings.TrimSpace(ref.Name)
			if name == "" {
				name = strings.TrimSpace(ref.DisplayPath)
			}
			path := strings.TrimSpace(ref.Path)
			if path == "" {
				path = strings.TrimSpace(ref.DisplayPath)
			}
			if name == "" || path == "" {
				continue
			}
			fmt.Fprintf(&sb, "   - name: %s\n", name)
			if desc := strings.TrimSpace(ref.Description); desc != "" {
				fmt.Fprintf(&sb, "     description: %s\n", desc)
			}
			if display := strings.TrimSpace(ref.DisplayPath); display != "" {
				fmt.Fprintf(&sb, "     display_path: %s\n", display)
			}
			fmt.Fprintf(&sb, "     path: %s\n", path)
			if ref.Missing {
				fmt.Fprintf(&sb, "     status: missing")
				if err := strings.TrimSpace(ref.Error); err != "" {
					fmt.Fprintf(&sb, " (%s)", err)
				}
				sb.WriteString("\n")
			}
		}
		for j, content := range rule.StyleContents {
			content = strings.TrimSpace(content)
			if content == "" {
				continue
			}
			fmt.Fprintf(&sb, "   Legacy inline style content %d:\n```markdown\n%s\n```\n", j+1, content)
		}
	}
	if wrote == 0 {
		return ""
	}
	sb.WriteString("\nTrigger rule: use prose-style references only for chapter prose creation, continuation, or rewriting, and for generating the next interactive-story turn. Global references apply to all prose generation by default. Before writing an interactive-story turn, use read to load every global reference path listed here. Choose scene-specific references only when they closely match the current chapter, interactive scene, or this turn's # scene selection; do not force a match. Before using a scene-specific path, read the actual file and use it only as guidance for voice, pacing, narration, sentence structure, and atmosphere. Do not copy its characters, plot, or setting.\n")
	sb.WriteString("Ignore these references for brainstorming, outlines, setting work, Q&A, planning, and other non-prose tasks. If no scene clearly matches, do not select a scene-specific reference.\n")
	return sb.String()
}

// ReferenceHeader precedes files referenced by the user.
const ReferenceHeader = "\n\n---\nFiles referenced by the user:\n"

// ReferenceOverflowHint reports that later references were omitted due to the size limit.
const ReferenceOverflowHint = "The total referenced content exceeded the limit; subsequent files were not read.\n"

// SelectionHeader precedes content selected in the editor.
const SelectionHeader = "\n\n---\nText selected by the user in the editor; operate on this content:\n"

// UnknownToolMessage is returned to the model when it calls an unavailable tool.
func UnknownToolMessage(name string) string {
	return fmt.Sprintf(
		"[tool error] Tool %q does not exist or is currently unavailable. Diagnose the error:\n"+
			"1) If the tool name is misspelled (for example, tod instead of todo), call the correct tool next.\n"+
			"2) If available tools cannot provide the capability, use another supported approach or answer the user in text.\n"+
			"3) Do not call the same nonexistent tool again.",
		name,
	)
}
