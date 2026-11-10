package lore

import (
	"fmt"
	"strings"
)

func formatLoreItemMarkdown(item Item, includeContent bool) string {
	var sb strings.Builder
	if !IsPublic(item) {
		sb.WriteString("作者裁决资料：不代表玩家或人物已知；仅按本局实际取得记录投影。\n\n")
	}
	fmt.Fprintf(&sb, "## %s (%s / %s / %s)\n\n", item.Name, item.Type, item.Importance, item.LoadMode)
	if item.ID != "" {
		fmt.Fprintf(&sb, "ID: %s\n", item.ID)
	}
	if len(item.Tags) > 0 {
		sb.WriteString("Tags: ")
		sb.WriteString(strings.Join(item.Tags, ", "))
		sb.WriteString("\n")
	}
	if item.BriefDescription != "" {
		sb.WriteString("Brief: ")
		sb.WriteString(item.BriefDescription)
		sb.WriteString("\n")
	}
	if includeContent {
		content := strings.TrimSpace(item.Content)
		if content != "" {
			sb.WriteString("\n")
			sb.WriteString(content)
		}
	}
	return strings.TrimSpace(sb.String())
}

// ReferenceMarkdown renders one complete lore item for Agent context and
// tool results. Keeping one renderer prevents context injection and explicit
// lore reads from presenting different source contracts to the model.
func ReferenceMarkdown(item Item) string {
	var sb strings.Builder
	if !IsPublic(item) {
		sb.WriteString("作者裁决资料：不代表玩家或人物已知；仅按本局实际取得记录投影。\n\n")
	}
	sb.WriteString("## ")
	sb.WriteString(item.Name)
	sb.WriteString(" (")
	sb.WriteString(item.Type)
	sb.WriteString(" / ")
	sb.WriteString(item.Importance)
	sb.WriteString(" / ")
	sb.WriteString(item.LoadMode)
	sb.WriteString(")\n")
	sb.WriteString("ID: ")
	sb.WriteString(item.ID)
	sb.WriteString("\n")
	if len(item.Tags) > 0 {
		sb.WriteString("Tags: ")
		sb.WriteString(strings.Join(item.Tags, ", "))
		sb.WriteString("\n")
	}
	if item.BriefDescription != "" {
		sb.WriteString("Brief: ")
		sb.WriteString(item.BriefDescription)
		sb.WriteString("\n")
	}
	content := strings.TrimSpace(item.Content)
	if content != "" {
		sb.WriteString("\n```markdown\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n")
	}
	return strings.TrimSpace(sb.String())
}
