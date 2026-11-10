package interactiveapp

import (
	"strings"
	"testing"

	"ta/internal/interactive"
	"ta/internal/publiquevent"
)

func TestTideGaugeNarratorDistinguishesMissedAndCompletedCalibration(t *testing.T) {
	event := interactive.PublicEventView{
		EventID:        publiquevent.PublicEventIDTideGauge,
		CurrentStageID: publiquevent.TideGaugeAfterRise,
		Actions:        []interactive.PublicEventActionView{{Kind: "investigate", Label: "复核外汊"}},
	}
	render := func() string {
		var page strings.Builder
		writeWorldEvents(&page, interactive.WorldState{Events: interactive.PublicEventState{
			Events: []interactive.PublicEventView{event},
		}}, interactive.StoryContextMaxBytes)
		return page.String()
	}
	page := render()
	if !strings.Contains(page, "当前分支错过校准，已无法取得短渡资格") ||
		!strings.Contains(page, "复核成功才开放十一小时外汊水路") ||
		!strings.Contains(page, "十一小时外汊是五个半时辰") ||
		!strings.Contains(page, "问路、打听或经过此地不等于参加") ||
		strings.Contains(page, "十小时短渡涨水后仍可出发") {
		t.Fatalf("missed window supplied incorrect route eligibility: %s", page)
	}

	event.Joined = true
	event.JoinedStage = publiquevent.TideGaugeBeforeRise
	event.Participation = "help"
	page = render()
	if !strings.Contains(page, "已列出的行路选项") || strings.Contains(page, "复核外汊") ||
		strings.Contains(page, "当前分支错过校准") {
		t.Fatalf("completed calibration presented a new recovery action: %s", page)
	}
}
