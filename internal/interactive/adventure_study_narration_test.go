package interactive

import (
	"strings"
	"testing"
)

func TestAdventureStudyNarrationUsesSettledMastery(t *testing.T) {
	rules := adventureTestRules()
	state := map[string]any{}
	for _, op := range adventureActorOps(NewAdventureActor(rules), "fixture") {
		applyActorStateOp(state, op)
	}
	context := TurnSubmissionContext{
		ActorState:   StoryDirectorActorStateSystem{AdventureRules: &rules},
		CurrentState: state,
		WorldView:    &TurnWorldView{},
	}
	for _, tc := range []struct {
		name  string
		hours int
		want  string
	}{
		{"complete", 2, "本次已学会"},
		{"partial", 1, "尚未学会"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := TurnSubmissionInput{
				World: &TurnWorldActionInput{Action: "adventure", SourceText: "研习进阶格斗", Adventure: &AdventureAction{Action: "learn", SkillID: "advanced", Hours: tc.hours}},
			}
			prepared, receipt := PrepareTurnSubmission(context, nil, input)
			if !receipt.NarrativeRewriteRequired || !strings.Contains(receipt.NarrativeRewriteReason, tc.want) || prepared.TurnResult().World == nil {
				t.Fatalf("study must retain the action and tell the narrator its outcome: %+v", receipt)
			}
			_, retry := PrepareTurnSubmission(context, prepared, input)
			if retry.NarrativeRewriteRequired {
				t.Fatalf("unchanged accepted study must not discard the corrected prose: %+v", retry)
			}
		})
	}
}
