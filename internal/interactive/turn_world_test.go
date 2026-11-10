package interactive

import (
	"strings"
	"testing"
)

// The world module is the one path a natural input takes to settle a journey, a
// task, a public event or a realm. These tests cover the decisions the module
// makes on its own: that it decodes, that it accepts exactly one action, that it
// refuses a second action rather than picking one, and that the shape checks
// match what each domain really needs. The domain answers themselves are the
// rules packages' and are asserted in their own suites.

func TestWorldModuleDecodesEachAction(t *testing.T) {
	cases := []struct {
		name  string
		input string
		check func(*testing.T, *TurnWorldActionInput)
	}{
		{
			name:  "journey",
			input: `{"world":{"action":"journey","route_id":"route_036:reverse","arrive":true}}`,
			check: func(t *testing.T, world *TurnWorldActionInput) {
				if world.Action != TurnWorldActionJourney || world.RouteID != "route_036:reverse" || !world.Arrive {
					t.Fatalf("journey did not decode: %#v", world)
				}
			},
		},
		{
			name:  "quest",
			input: `{"world":{"action":"quest","task_id":"quest.herb_handover","quest_action":"accept"}}`,
			check: func(t *testing.T, world *TurnWorldActionInput) {
				if world.TaskID != "quest.herb_handover" || world.QuestAction != QuestActionAccept {
					t.Fatalf("quest did not decode: %#v", world)
				}
			},
		},
		{
			name:  "public event",
			input: `{"world":{"action":"public_event","event_id":"evt.taiyue_spirit_tide","event_action":"join","participation_action":"help"}}`,
			check: func(t *testing.T, world *TurnWorldActionInput) {
				if world.EventID != "evt.taiyue_spirit_tide" || world.ParticipationAction != "help" {
					t.Fatalf("public event did not decode: %#v", world)
				}
			},
		},
		{
			name:  "realm",
			input: `{"world":{"action":"realm","realm_id":"realm.gu_xiu_dong_fu","realm_action":"enter"}}`,
			check: func(t *testing.T, world *TurnWorldActionInput) {
				if world.RealmID != "realm.gu_xiu_dong_fu" || world.RealmAction != RealmActionEnter {
					t.Fatalf("realm did not decode: %#v", world)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := DecodeInteractiveTurnSubmissionInput(tc.input)
			if len(input.Diagnostics) != 0 {
				t.Fatalf("unexpected diagnostics: %#v", input.Diagnostics)
			}
			if input.World == nil {
				t.Fatal("world module was dropped")
			}
			tc.check(t, input.World)
		})
	}
}

func TestWorldModuleRefusesTwoActionsInOneTurn(t *testing.T) {
	// A journey and a task in the same turn would settle two world facts from one
	// scene, and a journey would move a clock the task was accepted against. The
	// module refuses the shape rather than choosing one of them, and the refusal
	// is reported against /world so the model can drop the second action.
	input := DecodeInteractiveTurnSubmissionInput(
		`{"world":{"action":"journey","route_id":"route_036:reverse","arrive":true,"task_id":"quest.herb_handover","quest_action":"accept"}}`)
	if input.World != nil {
		t.Fatalf("a two-action module must not survive decoding: %#v", input.World)
	}
	var found *TurnSubmissionDiagnostic
	for i := range input.Diagnostics {
		if input.Diagnostics[i].Module == TurnSubmissionModuleWorld {
			found = &input.Diagnostics[i]
			break
		}
	}
	if found == nil || found.Path != "/world" {
		t.Fatalf("a second action must be diagnosed against /world: %#v", input.Diagnostics)
	}
}

func TestWorldModuleRequiresItsOwnFields(t *testing.T) {
	cases := []struct {
		name  string
		world TurnWorldActionInput
	}{
		{name: "journey without a distance", world: TurnWorldActionInput{Action: TurnWorldActionJourney}},
		{name: "quest without a task", world: TurnWorldActionInput{Action: TurnWorldActionQuest, QuestAction: QuestActionAccept}},
		{name: "quest without an action", world: TurnWorldActionInput{Action: TurnWorldActionQuest, TaskID: "quest.herb_handover"}},
		{name: "quest with an unknown action", world: TurnWorldActionInput{Action: TurnWorldActionQuest, TaskID: "quest.herb_handover", QuestAction: "pay"}},
		{name: "event without an event", world: TurnWorldActionInput{Action: TurnWorldActionPublicEvent, EventAction: PublicEventActionJoin, ParticipationAction: "help"}},
		{name: "event with an unknown participation", world: TurnWorldActionInput{Action: TurnWorldActionPublicEvent, EventID: "evt.taiyue_spirit_tide", EventAction: PublicEventActionJoin, ParticipationAction: "fight"}},
		{name: "realm without a realm", world: TurnWorldActionInput{Action: TurnWorldActionRealm, RealmAction: RealmActionEnter}},
		{name: "reward without a content ref", world: TurnWorldActionInput{Action: TurnWorldActionRealm, RealmAction: RealmActionReward, InstanceID: "rlm_1"}},
		{name: "wait without a duration", world: TurnWorldActionInput{Action: TurnWorldActionWait, SourceText: "等一天"}},
		{name: "wait without a player source", world: TurnWorldActionInput{Action: TurnWorldActionWait, DurationSeconds: 24 * 60 * 60}},
		{name: "unknown action", world: TurnWorldActionInput{Action: "travel"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			world := tc.world
			if diagnostic := validateTurnWorldActionModule(&world); diagnostic == nil {
				t.Fatalf("shape should have been refused: %#v", world)
			}
		})
	}
}

func TestWorldModuleAcceptsOneWellFormedActionOfEachKind(t *testing.T) {
	cases := map[string]TurnWorldActionInput{
		"journey arrive":                     {Action: TurnWorldActionJourney, RouteID: "route_036:reverse", Arrive: true},
		"journey partial":                    {Action: TurnWorldActionJourney, RouteID: "route_036:reverse", TargetMinutes: 10},
		"quest":                              {Action: TurnWorldActionQuest, TaskID: "quest.herb_handover", QuestAction: QuestActionSettle},
		"event join":                         {Action: TurnWorldActionPublicEvent, EventID: "evt.taiyue_spirit_tide", EventAction: PublicEventActionJoin, ParticipationAction: "observe"},
		"event join without a participation": {Action: TurnWorldActionPublicEvent, EventID: "evt.taiyue_spirit_tide", EventAction: PublicEventActionJoin},
		"event withdraw":                     {Action: TurnWorldActionPublicEvent, EventID: "evt.taiyue_spirit_tide", EventAction: PublicEventActionWithdraw},
		"realm enter":                        {Action: TurnWorldActionRealm, RealmID: "realm.gu_xiu_dong_fu", RealmAction: RealmActionEnter},
		"realm move":                         {Action: TurnWorldActionRealm, InstanceID: "rlm_1", RealmAction: RealmActionMove, ConnectionID: "realm.hall_to_plot", DestinationAreaID: "realm.herb_plot"},
		"realm reward":                       {Action: TurnWorldActionRealm, InstanceID: "rlm_1", RealmAction: RealmActionReward, ContentRefID: "realm.herb_ref"},
		"realm exit":                         {Action: TurnWorldActionRealm, InstanceID: "rlm_1", RealmAction: RealmActionExit},
		"wait":                               {Action: TurnWorldActionWait, DurationSeconds: 24 * 60 * 60, SourceText: "等一天"},
	}
	for name, world := range cases {
		t.Run(name, func(t *testing.T) {
			action := world
			if diagnostic := validateTurnWorldActionModule(&action); diagnostic != nil {
				t.Fatalf("a well-formed action was refused: %#v", diagnostic)
			}
		})
	}
}

func TestWaitMustQuoteTheCurrentPlayerInput(t *testing.T) {
	world := TurnWorldActionInput{Action: TurnWorldActionWait, DurationSeconds: 24 * 60 * 60, SourceText: "等一天"}
	choices := testTurnChoices()
	_, good := PrepareTurnSubmission(TurnSubmissionContext{CurrentPlayerInput: "我在驿站等一天"}, nil,
		TurnSubmissionInput{World: &world, Choices: &choices})
	if good.ModuleStatus.World != TurnSubmissionModuleAccepted {
		t.Fatalf("wait with a matching player quote was refused: %#v", good)
	}
	_, bad := PrepareTurnSubmission(TurnSubmissionContext{CurrentPlayerInput: "我今天直接动身"}, nil,
		TurnSubmissionInput{World: &world, Choices: &choices})
	if bad.ModuleStatus.World == TurnSubmissionModuleAccepted {
		t.Fatalf("wait without a matching player source was accepted: %#v", bad)
	}
}

func TestWorldModuleIsOmittedWhenEmpty(t *testing.T) {
	// The common turn settles no world action, and an omitted module must not read
	// as an empty action that the commit then tries to plan.
	var world TurnWorldActionInput
	if !world.Empty() {
		t.Fatal("a zero module must read as empty")
	}
	world.Action = TurnWorldActionJourney
	if world.Empty() {
		t.Fatal("a module naming an action is not empty")
	}
}

func TestWorldModuleOwnsTheSceneLocationOnlyWhereItWritesIt(t *testing.T) {
	// A journey moves the character and a realm exit restores where they entered
	// from, so both write 当前详细地点 and the turn must not also write it. A task,
	// a participation and an entry into a realm do not move the scene field.
	journey := TurnWorldActionInput{Action: TurnWorldActionJourney, RouteID: "route_036:reverse", Arrive: true}
	if !turnWorldOwnsSceneLocation(&journey) {
		t.Fatal("a journey writes the scene location")
	}
	exit := TurnWorldActionInput{Action: TurnWorldActionRealm, RealmAction: RealmActionExit}
	if !turnWorldOwnsSceneLocation(&exit) {
		t.Fatal("a realm exit writes the scene location")
	}
	for name, world := range map[string]TurnWorldActionInput{
		"quest":       {Action: TurnWorldActionQuest, TaskID: "quest.herb_handover", QuestAction: QuestActionAccept},
		"event":       {Action: TurnWorldActionPublicEvent, EventID: "evt.taiyue_spirit_tide", EventAction: PublicEventActionJoin, ParticipationAction: "help"},
		"realm enter": {Action: TurnWorldActionRealm, RealmID: "realm.gu_xiu_dong_fu", RealmAction: RealmActionEnter},
	} {
		t.Run(name, func(t *testing.T) {
			action := world
			if turnWorldOwnsSceneLocation(&action) {
				t.Fatalf("%s does not write the scene location", name)
			}
		})
	}
}

func TestWorldModuleCannotShareATurnWithRestOrTraining(t *testing.T) {
	// The submission keeps them exclusive, so the model cannot settle a journey
	// and a closed-door session in the same scene. This asserts the receipt says
	// so rather than silently keeping one.
	system, state := turnSubmissionTestState()
	choices := testTurnChoices()
	world := TurnWorldActionInput{Action: TurnWorldActionJourney, RouteID: "route_036:reverse", Arrive: true}
	cultivation := TurnCultivationInput{Days: 1}
	input := TurnSubmissionInput{World: &world, Cultivation: &cultivation, Choices: &choices}
	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, input)
	if receipt.ModuleStatus.World == TurnSubmissionModuleAccepted && receipt.ModuleStatus.Cultivation == TurnSubmissionModuleAccepted {
		t.Fatalf("a world action and training must not both be accepted: %#v", receipt.ModuleStatus)
	}
}

func TestWorldModuleDiagnosticNamesTheOffendingField(t *testing.T) {
	world := TurnWorldActionInput{Action: TurnWorldActionJourney}
	diagnostic := validateTurnWorldActionModule(&world)
	if diagnostic == nil || diagnostic.Path != "/world/arrive" || !strings.Contains(diagnostic.Expected, "arrive=true") {
		t.Fatalf("a journey without a distance must name the field it needs: %#v", diagnostic)
	}
}
