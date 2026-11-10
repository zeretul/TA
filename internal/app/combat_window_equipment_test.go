package app

import (
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// The tests here cover what the battle window tells the interface about the
// five positions of 《自制-装备栏与法宝御使》§1 and the spares a mid-fight change
// may take up.
//
// They go through PlayCombatRound and read the window the interface reads, so
// the payload is checked as it is really delivered rather than as a helper
// assembled it.

// windowEquipmentSystem builds the xiuxian system with the player already
// wearing a given set and carrying a given pack.
func windowEquipmentSystem(t *testing.T, worn, pack map[string]any) interactive.StoryDirectorActorStateSystem {
	t.Helper()
	module, err := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.ActorStateXiuxianID)
	if err != nil {
		t.Fatalf("load the xiuxian module: %v", err)
	}
	system := module.ActorState
	rival := system.Templates[0]
	rival.ID = "rival"
	rival.Name = "对手"
	system.Templates = append(system.Templates, rival)

	stage := cultivation.StageKey(cultivation.Qi(7))
	state := map[string]any{combatTestRealmField: stage}
	for key, value := range worn {
		state[key] = value
	}
	if len(pack) > 0 {
		state[combatTestCarriedField] = pack
	}
	system.InitialActors = append(system.InitialActors, interactive.ActorStateInitialActor{
		ID: "rival", Name: "对手", TemplateID: "rival",
		State: map[string]any{combatTestRealmField: stage},
	})
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != interactive.DefaultActorID {
			continue
		}
		system.InitialActors[index].State = state
	}
	return system
}

// windowWornEntry records one position the way equipping writes it.
func windowWornEntry(defID, name string) map[string]any {
	return map[string]any{
		"名称": name, "定义": defID,
		"品级":  map[string]string{"implement.lower.common": "下品", "implement.middle.rare": "中品"}[defID],
		"稀有度": map[string]string{"implement.lower.common": "常见", "implement.middle.rare": "稀有"}[defID],
	}
}

// §1: the window reports all five positions, including the empty ones.
//
// The rule allows a weapon, a garment and three treasures. Reporting only the
// occupied ones would let the interface draw a shorter bar than the rules allow,
// and the player would not know a position was free.
func TestCombatWindowReportsEveryEquipmentPosition(t *testing.T) {
	application, storyID := combatTestAppWithSystem(t, windowEquipmentSystem(t,
		map[string]any{combatTestImplementField: windowWornEntry("implement.middle.rare", "青锋剑")},
		nil,
	))
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}

	player := combatWindowSide(t, window, interactive.DefaultActorID)
	if len(player.Equipment) != len(cultivation.SwappableEquipmentSlots) {
		t.Fatalf("the window must report all %d positions, got %d",
			len(cultivation.SwappableEquipmentSlots), len(player.Equipment))
	}
	bySlot := map[string]CombatWindowEquipment{}
	for _, position := range player.Equipment {
		bySlot[position.Slot] = position
	}
	weapon := bySlot[string(cultivation.EquipmentSlotWeapon)]
	if weapon.Name != "青锋剑" {
		t.Errorf("the weapon position = %q, want 青锋剑", weapon.Name)
	}
	if weapon.InstanceID != "青锋剑" {
		t.Errorf("an occupied position must name the copy that holds it, got %q", weapon.InstanceID)
	}
	if weapon.DefID != "implement.middle.rare" {
		t.Errorf("the position must report the registered definition, got %q", weapon.DefID)
	}
	// The empty ones are present and empty, not absent.
	for _, slot := range []cultivation.EquipmentSlot{
		cultivation.EquipmentSlotGarment,
		cultivation.EquipmentSlotAttackTreasure,
		cultivation.EquipmentSlotDefenseTreasure,
		cultivation.EquipmentSlotSupportTreasure,
	} {
		position, ok := bySlot[string(slot)]
		if !ok {
			t.Errorf("the %s position must be reported even when empty", slot)
			continue
		}
		if position.Name != "" || position.InstanceID != "" {
			t.Errorf("the %s position must be empty, got %#v", slot, position)
		}
	}
}

// §1: a pack copy is reported as a spare and carries no number.
func TestCombatWindowReportsPackCopiesAsSpares(t *testing.T) {
	application, storyID := combatTestAppWithSystem(t, windowEquipmentSystem(t, nil,
		map[string]any{
			"备用剑": map[string]any{"定义": "implement.lower.common", "数量": float64(1)},
		},
	))
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}

	player := combatWindowSide(t, window, interactive.DefaultActorID)
	if len(player.Spares) != 1 {
		t.Fatalf("the pack copy must be offered as one spare, got %d", len(player.Spares))
	}
	spare := player.Spares[0]
	if spare.InstanceID != "备用剑" {
		t.Errorf("the spare must carry the pack key as its identity, got %q", spare.InstanceID)
	}
	if spare.Slot != string(cultivation.EquipmentSlotWeapon) {
		t.Errorf("an implement spare must name the weapon position, got %q", spare.Slot)
	}
	// The spare payload has no field for a number, which is §1's 放在包里不加属性
	// made structural: the interface cannot show a bonus the pack does not give.
	if spare.Name != "备用剑" {
		t.Errorf("the spare must carry its name, got %q", spare.Name)
	}
}

// §三.1: an opponent's positions are not the player's to read.
func TestCombatWindowKeepsTheOpponentsEquipmentOutOfSight(t *testing.T) {
	application, storyID := combatTestAppWithSystem(t, windowEquipmentSystem(t,
		map[string]any{combatTestImplementField: windowWornEntry("implement.middle.rare", "青锋剑")},
		nil,
	))
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}

	rival := combatWindowSide(t, window, "rival")
	if len(rival.Equipment) != 0 || len(rival.Spares) != 0 {
		t.Errorf("an opponent's positions and pack must not be reported, got %#v / %#v",
			rival.Equipment, rival.Spares)
	}
}

// §2: a change of equipment is a real action, so the window accepts it and the
// positions it produced are what the next read reports.
func TestCombatWindowAppliesAChangeOfEquipment(t *testing.T) {
	application, storyID := combatTestAppWithSystem(t, windowEquipmentSystem(t,
		map[string]any{combatTestImplementField: windowWornEntry("implement.lower.common", "旧剑")},
		map[string]any{"新剑": map[string]any{"定义": "implement.middle.rare", "数量": float64(1)}},
	))
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	before := combatWindowSide(t, opened, interactive.DefaultActorID)
	attackBefore := 0
	if before.Attack != nil {
		attackBefore = *before.Attack
	}

	after, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-swap",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionSwapEquipment, SwapInstanceID: "新剑"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	player := combatWindowSide(t, after, interactive.DefaultActorID)
	bySlot := map[string]CombatWindowEquipment{}
	for _, position := range player.Equipment {
		bySlot[position.Slot] = position
	}
	if got := bySlot[string(cultivation.EquipmentSlotWeapon)].Name; got != "新剑" {
		t.Errorf("the weapon position after the change = %q, want 新剑", got)
	}
	if player.Attack == nil {
		t.Fatal("the window must report the player's attack")
	}
	if *player.Attack <= attackBefore {
		t.Errorf("attack after swapping a 5%% implement for a 15%% one = %d, want more than %d",
			*player.Attack, attackBefore)
	}
	// The copy that came out is a spare now, not a lost object.
	spares := map[string]bool{}
	for _, spare := range player.Spares {
		spares[spare.InstanceID] = true
	}
	if !spares["旧剑"] {
		t.Errorf("the copy that was put away must be offered as a spare, got %v", spares)
	}
	if spares["新剑"] {
		t.Error("the copy taken up must no longer be offered as a spare")
	}
}

// §2: a change naming nothing is refused rather than guessed at.
func TestCombatWindowRefusesAChangeOfEquipmentWithoutASpare(t *testing.T) {
	application, storyID := combatTestAppWithSystem(t, windowEquipmentSystem(t, nil,
		map[string]any{"新剑": map[string]any{"定义": "implement.middle.rare", "数量": float64(1)}},
	))
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-swap-none",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionSwapEquipment},
		},
	}); err == nil {
		t.Fatal("a change that names no spare must be refused")
	}
}
