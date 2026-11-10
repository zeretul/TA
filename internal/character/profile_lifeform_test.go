package character

import "testing"

func TestProfileLifeformAndMethodPersistAndCannotChange(t *testing.T) {
	state := State{}
	initial := Profile{
		ID: "gray", StoryID: "story", Name: "灰团", Revision: 1,
		Lifeform: LifeformSpiritBeast, CultivationMethodID: "method.spirit_beast_breathing",
	}
	if err := state.UpsertProfile(initial); err != nil {
		t.Fatal(err)
	}
	if err := state.UpsertProfile(Profile{ID: "gray", StoryID: "story", Name: "灰团", Revision: 1}); err != nil {
		t.Fatalf("ordinary update erased registered qualification: %v", err)
	}
	stored, ok := state.Profile("gray")
	if !ok || stored.Lifeform != LifeformSpiritBeast || stored.CultivationMethodID != initial.CultivationMethodID {
		t.Fatalf("registered qualification did not persist: %+v", stored)
	}
	if err := state.UpsertProfile(Profile{ID: "gray", StoryID: "story", Name: "灰团", Revision: 1, Lifeform: LifeformAnimal}); err == nil {
		t.Fatal("lifeform reclassification was accepted")
	}
	if err := state.UpsertProfile(Profile{ID: "gray", StoryID: "story", Name: "灰团", Revision: 1, Lifeform: LifeformSpiritBeast, CultivationMethodID: "method.other"}); err == nil {
		t.Fatal("cultivation method change was accepted")
	}
}
