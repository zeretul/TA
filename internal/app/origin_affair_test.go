package app

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ta/config"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/quest"
	"ta/internal/storypack"
)

// Keep every real origin's zero-funded inventory and responsibility. Only the
// opening scene description is supplied, then actual registered routes are used.
func TestAllSpaceOriginsMayAcceptOrRefuseSharedBerthHelpWithoutInheritingDebt(t *testing.T) {
	for _, spec := range []struct {
		id, start string
		route     []string
	}{{"xinghai-c01", "xinghai-l01", nil}, {"xinghai-c02", "xinghai-l06", []string{"xinghai-l04", "xinghai-l01"}}, {"xinghai-c03", "xinghai-l17", []string{"xinghai-l16", "xinghai-l04", "xinghai-l01"}}} {
		t.Run(spec.id, func(t *testing.T) {
			root := t.TempDir()
			application := &App{workspace: root, cfg: &config.Config{DenovaDir: root}}
			service := &InteractiveAppService{app: application}
			pack, e := storypack.NewLibrary(root).Get("xinghai-689")
			if e != nil {
				t.Fatal(e)
			}
			req, _, e := pack.StoryRequestForTemplate(spec.id)
			if e != nil {
				t.Fatal(e)
			}
			req.PackTemplate = &interactive.StoryProtagonistTemplateRef{PackID: pack.ID, TemplateID: spec.id}
			req, e = service.withStoryCreationDefaults(req)
			if e != nil {
				t.Fatal(e)
			}
			req, e = service.withPackTemplate(req)
			if e != nil {
				t.Fatal(e)
			}
			s := interactive.NewStore(root)
			defer func() { _ = s.Close() }()
			application.interactive = s
			story, e := s.CreateStory(req)
			if e != nil {
				t.Fatal(e)
			}
			place := ""
			for _, w := range pack.World {
				if w.ID == spec.start {
					place = w.Name
				}
			}
			_, _, e = s.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "观察周围", Narrative: "你确认了自己的所在。", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "休息"}, StateUpdates: []interactivestate.Update{{Op: "replace", Path: "/story/当前详细地点", Value: place}}}})
			if e != nil {
				t.Fatal(e)
			}
			sequence := 0
			command := func() string { sequence++; return fmt.Sprint("origin-", sequence) }
			for _, destination := range spec.route {
				routes, e := s.JourneyOptionsFor(story.ID, "main")
				if e != nil {
					t.Fatal(e)
				}
				arrived := false
				for _, route := range routes.Routes {
					if route.DestinationID == destination {
						_, e = s.ApplyJourney(story.ID, interactive.JourneyRequest{BranchID: "main", RouteID: route.RouteID, Arrive: true, CommandID: command(), CommandHash: interactive.ContentHashOf(route.RouteID)})
						if e != nil {
							t.Fatal(e)
						}
						arrived = true
						break
					}
				}
				if !arrived {
					t.Fatal("actual route missing", destination)
				}
			}
			checkpoint, _, e := s.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "查看四十七号泊位账单", Narrative: "你核对了这笔泊位费用的说明，尚未答应帮工。", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "休息"}}})
			if e != nil {
				t.Fatal(e)
			}
			initial, e := s.AdventureWindow(story.ID, "main")
			if e != nil {
				t.Fatal(e)
			}
			if spec.id != "xinghai-c01" && len(initial.Actor.OpeningHazards) != 0 {
				t.Fatal("nondebtor inherited the worker's personal hazard")
			}
			view, e := application.QuestStateForLocale(story.ID, "main", "en-US")
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, op := range view.Available {
				if op.TaskID == "xinghai-a01" {
					found = true
					if !strings.Contains(op.Brief, "voluntary") || strings.Contains(strings.ToLower(op.Brief), "your debt") {
						t.Fatal("public offer fabricated a universal personal debt")
					}
				}
			}
			if !found {
				t.Fatal("shared local help not discoverable")
			}
			unseen, e := s.QuestStateFor(story.ID, "main")
			if e != nil {
				t.Fatal(e)
			}
			for _, task := range unseen.Tasks {
				if task.TaskID == "xinghai-a01" {
					t.Fatal("reading the voluntary offer accepted or created it")
				}
			}
			afterRead, e := s.AdventureWindow(story.ID, "main")
			if e != nil || afterRead.Head != initial.Head {
				t.Fatal("reading an offer mutated canonical head", e)
			}
			fork, e := s.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: checkpoint.ID, Title: "Decline the shared job"})
			if e != nil {
				t.Fatal(e)
			}
			apply := func(branch, action string) interactive.QuestResult {
				t.Helper()
				out, e := s.ApplyQuest(story.ID, interactive.QuestRequest{BranchID: branch, TaskID: "xinghai-a01", Action: action, CommandID: command(), CommandHash: interactive.ContentHashOf([]string{branch, action})})
				if e != nil {
					t.Fatal(e)
				}
				return out
			}
			offered := apply(fork.ID, interactive.QuestActionOffer)
			if offered.Instance.DeadlineSeconds != 0 {
				t.Fatal("mere offer started an agreed delivery deadline")
			}
			apply(fork.ID, interactive.QuestActionAbandon)
			accepted := apply("main", interactive.QuestActionAccept)
			if accepted.Instance.Status != quest.StatusAccepted || accepted.Instance.DeadlineSeconds != initial.ClockSeconds+7200 {
				t.Fatal("accepted shared work did not bind its actual two-hour delivery agreement")
			}
			for _, branch := range []string{"main", fork.ID} {
				before, e := s.AdventureWindow(story.ID, branch)
				if e != nil {
					t.Fatal(e)
				}
				if !reflect.DeepEqual(before.Actor, initial.Actor) || before.ClockSeconds != initial.ClockSeconds {
					t.Fatal("accept/refuse moved resources, time, personal responsibility or hazard")
				}
				target := initial.ClockSeconds + 10800
				for now := before.ClockSeconds; now < target; {
					text := fmt.Sprintf("等候%d秒", target-now)
					_, _, e = s.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: branch, User: text, Narrative: "你按自己的决定继续等候。", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "休息"}, World: &interactive.TurnWorldActionInput{Action: "wait", DurationSeconds: target - now, SourceText: text}}})
					if e != nil {
						t.Fatal(e)
					}
					w, e := s.AdventureWindow(story.ID, branch)
					if e != nil || w.ClockSeconds <= now {
						t.Fatal("authorized wait did not advance formal time", e)
					}
					now = w.ClockSeconds
				}
				after, e := s.AdventureWindow(story.ID, branch)
				if e != nil {
					t.Fatal(e)
				}
				if spec.id != "xinghai-c01" && (after.Actor.Health != initial.Actor.Health || after.Actor.Injury != initial.Actor.Injury || len(after.Actor.OpeningHazards) != 0 || after.Actor.Currency != initial.Actor.Currency || after.Actor.Experience != initial.Actor.Experience || !reflect.DeepEqual(after.Actor.Inventory, initial.Actor.Inventory) || !reflect.DeepEqual(after.Actor.Skills, initial.Actor.Skills)) {
					t.Fatal("shared-work deadline/refusal invented the worker's personal debt or vacuum consequence")
				}
				if spec.id == "xinghai-c01" {
					h := initial.Actor.OpeningHazards[0]
					if after.Actor.Health != initial.Actor.Health-h.Damage || after.Actor.Injury != initial.Actor.Injury+h.Injury || len(after.Actor.OpeningHazards) != 1 || !after.Actor.OpeningHazards[0].Resolved || after.Actor.OpeningHazards[0].Outcome != "exposed" {
						t.Fatal("shared task acceptance/refusal erased the debtor origin's independent real hazard")
					}
				}
				state, e := s.QuestStateFor(story.ID, branch)
				if e != nil {
					t.Fatal(e)
				}
				want := string(quest.StatusExpired)
				if branch == fork.ID {
					want = string(quest.StatusAbandoned)
				}
				for _, task := range state.Tasks {
					if task.TaskID == "xinghai-a01" && task.Status != want {
						t.Fatal("refusal or accepted deadline lost its own branch status", task.Status, want)
					}
				}
				for _, op := range state.Available {
					if op.TaskID == "xinghai-a01-echo-3" {
						t.Fatal("nonparticipant inherited a forgery-payment followup")
					}
				}
			}
			before, e := s.AdventureWindow(story.ID, "main")
			if e != nil {
				t.Fatal(e)
			}
			if e = s.Close(); e != nil {
				t.Fatal(e)
			}
			s = interactive.NewStore(root)
			application.interactive = s
			after, e := s.AdventureWindow(story.ID, "main")
			if e != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("cold replay changed actual responsibility or state", e)
			}
		})
	}
}
