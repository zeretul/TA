package tools

import (
	"encoding/json"
	"strings"
	agent "ta/agent"
	"ta/internal/interactive"
	"testing"
)

func TestSubmitTurnExposesSourceBoundVisualKnowledge(t *testing.T) {
	info, err := agent.GoStruct2ToolInfo[submitInteractiveTurnToolSchema](submitInteractiveTurnToolName, "test")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"visual", "subject_id", "contact_exchange_index", "recognition_exchange_index", "不能为此编造话语", "不是显示图片的许可"} {
		if !strings.Contains(string(raw), text) {
			t.Fatalf("actual model tool schema lacks %q", text)
		}
	}
	input := interactive.DecodeInteractiveTurnSubmissionInput(`{"character_facts":{"knowledge":[{"character_id":"protagonist","statement":"实际亲眼观察","nature":"experienced","channel":"scene","visual":{"subject_id":"sinistra","form":"人类女性","contact_exchange_index":1,"recognition_exchange_index":2}}]}}`)
	if len(input.Diagnostics) > 0 || input.CharacterFacts == nil || len(input.CharacterFacts.Knowledge) != 1 || input.CharacterFacts.Knowledge[0].Visual == nil || input.CharacterFacts.Knowledge[0].Visual.RecognitionExchangeIndex != 2 {
		t.Fatalf("actual tool decoder dropped visual relation: %+v", input)
	}
}
