package interactiveapp

import (
	"encoding/json"
	"fmt"
	"strings"
	"ta/internal/interactive"
)

// The ordinary world page only consumes the already-public view. Private
// author causes are injected separately and never passed through this helper.
func writeWorldLocalAffairs(sb *strings.Builder, affairs []interactive.LocalAffairView, maxBytes int) {
	if len(affairs) == 0 {
		return
	}
	sb.WriteString("\n### 已遇见的地方小事（可暂放）\n")
	for _, a := range affairs[max(0, len(affairs)-12):] {
		data, _ := json.Marshal(a)
		entry := fmt.Sprintf("- %s\n", data)
		if sb.Len()+len(entry) > maxBytes {
			break
		}
		sb.WriteString(entry)
	}
}
