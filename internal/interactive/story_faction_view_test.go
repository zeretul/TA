package interactive

import (
	"reflect"
	"testing"

	"ta/internal/exploration"
)

func TestFactionAffairViewIsReadOnlyAndHidesUnavailableWork(t *testing.T) {
	s, id, recipe, _ := factionFlowStory(t, "lanchuan-qunxia", "lanchuan-prepare-salve", "lanchuan-p15")
	req := factionFlowRequest(t, s, id, "main", "view-start", "start", "view", recipe.ID, 0)
	if _, _, err := s.AppendTurnWithState(id, req); err != nil {
		t.Fatal(err)
	}
	p, meta := cooperationViewReadFixture(t, s, id)
	before := cloneStoryState(p.State)
	chars := cloneCharacterState(p.Characters)
	shown := factionAffairsView(p, meta)
	if len(shown.Projects) != 1 || !shown.Projects[0].CanWork || len(shown.Recipes) == 0 {
		t.Fatal("known present actor lost the formal affair")
	}
	if !reflect.DeepEqual(before, p.State) || !reflect.DeepEqual(chars, p.Characters) {
		t.Fatal("reading affair changed committed resources or character history")
	}
	p.Realms = map[string]RealmInstanceState{"inside": {Snapshot: exploration.DungeonInstanceSnapshot{Presence: exploration.InstanceInside}}}
	inside := factionAffairsView(p, meta)
	if inside.Projects[0].CanWork || len(inside.Recipes) != 0 {
		t.Fatal("exploration visit offered unavailable production")
	}
	p.Realms = nil
	applyActorStateOp(p.State, ActorStateOp{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{}})
	away := factionAffairsView(p, meta)
	if away.Projects[0].CanWork || len(away.Recipes) != 0 {
		t.Fatal("read brought absent person back or offered remote work")
	}
	p.Characters = nil
	if unknown := factionAffairsView(p, meta); len(unknown.Projects) != 0 || len(unknown.Recipes) != 0 {
		t.Fatal("unencountered catalog person became an offered worker")
	}
}
