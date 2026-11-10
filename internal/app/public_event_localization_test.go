package app

import (
	"encoding/json"
	"strings"
	"ta/internal/interactive"
	"ta/internal/publiquevent"
	"testing"
	"unicode"
)

func TestTideGaugeEnglishReadPreservesVisibilityAndRules(t *testing.T) {
	for _, eventID := range []string{publiquevent.PublicEventIDTideGauge, publiquevent.PublicEventIDTaiyueTide} {
		d, _ := publiquevent.DefinitionByID(eventID)
		for _, stage := range d.Stages {
			for _, joined := range []bool{false, true} {
				lines := []string{stage.Summary}
				if joined {
					lines = append(lines, stage.ParticipantLines...)
					lines = append(lines, stage.SecretLines...)
				}
				view := PublicEventAppState{ClockSeconds: stage.OpenAfterSeconds, Events: []interactive.PublicEventView{{EventID: d.ID, EventVersion: d.Version, Title: d.Title, Brief: d.Brief, Region: d.Region, CurrentStageID: stage.ID, CurrentStageTitle: stage.Title, Joined: joined, Stages: []interactive.PublicEventStageView{{ID: stage.ID, Title: stage.Title, Status: "current", Lines: lines, OpensAtSeconds: stage.OpenAfterSeconds}}}}}
				for _, p := range d.Participants {
					view.Events[0].Participants = append(view.Events[0].Participants, interactive.PublicEventParticipantView{ID: p.ID, Name: p.Name, Role: p.PublicRole})
				}
				for _, a := range d.Actions {
					view.Events[0].Actions = append(view.Events[0].Actions, interactive.PublicEventActionView{Label: a.Label, Brief: a.Brief})
				}
				before, _ := json.Marshal(view)
				en := localizePublicEventState(view, "en-US")
				after, _ := json.Marshal(view)
				if string(before) != string(after) {
					t.Fatal("display read changed its source")
				}
				e := en.Events[0]
				if e.EventID != d.ID || e.CurrentStageID != stage.ID || e.Joined != joined || len(e.Stages) != 1 || len(e.Stages[0].Lines) != len(lines) || en.ClockSeconds != view.ClockSeconds {
					t.Fatal("English read changed visibility or rule state")
				}
				if e.Stages[0].OpensAtSeconds != stage.OpenAfterSeconds || e.Stages[0].Status != "current" {
					t.Fatal("stage timing changed")
				}
				extra := []string{}
				for _, p := range e.Participants {
					extra = append(extra, p.Name, p.Role)
				}
				for _, a := range e.Actions {
					extra = append(extra, a.Label, a.Brief)
				}
				extra = append(extra, e.Stages[0].Lines...)
				for _, text := range append([]string{e.Title, e.Brief, e.Region, e.CurrentStageTitle}, extra...) {
					if strings.ContainsFunc(text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
						t.Fatalf("untranslated visible prose: %s", text)
					}
				}
			}
		}
	}
}
