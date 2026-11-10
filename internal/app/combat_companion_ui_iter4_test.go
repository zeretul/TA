package app

import (
	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"testing"
)

func TestIter4CombatWindowDoesNotPromoteCompanionToProtagonist(t *testing.T) {
	ally := cultivation.Combatant{ID: "青焰狐", Side: cultivation.SidePlayer, Skills: []cultivation.SkillDef{{ID: "beast_breath"}}}
	player := cultivation.Combatant{ID: interactive.DefaultActorID, Side: cultivation.SidePlayer, Skills: []cultivation.SkillDef{{ID: "fire_bolt"}}}
	session := cultivation.CombatSession{Combatants: []cultivation.Combatant{ally, player}}
	if got := combatWindowPlayerActorID(session); got != interactive.DefaultActorID {
		t.Fatalf("UI assigned protagonist controls to %q", got)
	}
	skills := combatWindowPlayerSkills(session)
	if _, ok := skills["beast_breath"]; ok {
		t.Fatal("companion's private skills became protagonist knowledge")
	}
	if _, ok := skills["fire_bolt"]; !ok {
		t.Fatal("protagonist's actual skills were hidden")
	}
	session.Combatants = []cultivation.Combatant{ally}
	if got := combatWindowPlayerActorID(session); got != "" {
		t.Fatalf("missing protagonist reassigned control to %q", got)
	}
	if got := combatWindowPlayerSkills(session); len(got) != 0 {
		t.Fatalf("missing protagonist inherited companion skills: %+v", got)
	}
}

func TestIter4CombatCompanionUsesObservedNotPrivateProjection(t *testing.T) {
	session := cultivation.CombatSession{Combatants: []cultivation.Combatant{
		{ID: "青焰狐", Side: cultivation.SidePlayer, Skills: []cultivation.SkillDef{{ID: "secret_breath"}}},
		{ID: interactive.DefaultActorID, Side: cultivation.SidePlayer},
	}}
	observations := combatWindowObservations(session, nil, combatWindowPlayerSkills(session))
	sides := combatWindowSides(session, character.State{}, observations)
	if len(sides) != 2 {
		t.Fatalf("sides: %+v", sides)
	}
	ally := sides[0]
	if ally.Side != "player" || ally.Exact || ally.Health != nil || ally.Mana != nil || len(ally.Skills) != 0 || len(ally.Items) != 0 || len(ally.Equipment) != 0 {
		t.Fatalf("ally assistance revealed private dossier: %+v", ally)
	}
	if observations["青焰狐"] == nil || !sides[1].Exact {
		t.Fatal("ally must remain visibly observed while the protagonist remains exact")
	}
	if combatWindowPlayerSide(session)["青焰狐"] {
		t.Fatal("history filtering treats ally's hidden actions as protagonist's own knowledge")
	}
}
