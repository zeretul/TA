package app

import (
	"strings"
	"testing"

	"ta/internal/book/lore"
	"ta/internal/character"
)

// The two packs below are deliberately adversarial: they use the same place
// name and the same character name, and they say contradictory things about
// both. That is the shape a real workspace has once a user installs a second
// world, and it is the shape a scope bug turns into a wrong answer rather than
// a crash. The assertions are about what a character can read, because that is
// where a leak would become visible in play: a character answering from another
// world's resident lore is speaking from knowledge they never had a way to
// acquire.
const (
	sameNamePlace = "青石镇"
	sameNameActor = "周掌柜"
)

func seedTwoConflictingWorlds(t *testing.T) (workspace, alphaScope, betaScope string) {
	t.Helper()
	workspace = t.TempDir()
	seeder := lore.NewStore(workspace)
	alphaScope = "pack:alpha@rev1"
	betaScope = "pack:beta@rev1"

	seed := func(scope, id, name, content, loadMode string) {
		t.Helper()
		if _, err := seeder.Create(lore.ItemInput{
			ID: id, Scope: scope, Type: "setting", Name: name, Content: content, LoadMode: loadMode,
		}); err != nil {
			t.Fatalf("seed %s/%s: %v", scope, id, err)
		}
	}

	// Alpha: the town is a river port and the shopkeeper is the one who decides.
	seed(alphaScope, "alpha-town", sameNamePlace, "阿尔法世界的青石镇：一座河港，靠水运为生。", lore.LoadModeResident)
	seed(alphaScope, "alpha-actor", sameNameActor, "阿尔法世界的周掌柜：河港最大的粮商，说一不二。", lore.LoadModeAuto)
	// Beta: the same names, a contradictory world — an inland mining town, and a
	// shopkeeper who answers to someone else.
	seed(betaScope, "beta-town", sameNamePlace, "贝塔世界的青石镇：一座内陆矿镇，以采银为生。", lore.LoadModeResident)
	seed(betaScope, "beta-actor", sameNameActor, "贝塔世界的周掌柜：矿主雇来的管事，凡事要请示东家。", lore.LoadModeAuto)

	return workspace, alphaScope, betaScope
}

// TestCharacterLoreNeverCrossesWorldsWithSameNames is the same-name isolation
// rule at the character seam. Two packs define a place and a person with
// identical names and incompatible content; a character bound to one pack must
// be able to read only their own world's version.
//
// The earlier acceptance only proved that two stories do not share a cast. That
// is a weaker claim: it would still pass if the lore reader ignored scope and
// simply happened to be asked about a name only one world defined. This test
// makes the names collide on purpose, so scope is the only thing that can
// separate the two answers.
func TestCharacterLoreNeverCrossesWorldsWithSameNames(t *testing.T) {
	workspace, alphaScope, betaScope := seedTwoConflictingWorlds(t)

	// The character's own profile and scene name the shared place and person, so
	// grounding matches both worlds' entries. Only the scope decides which is
	// delivered.
	profile := character.Profile{
		ID: "zhou", StoryID: "st", Name: sameNameActor, Role: "掌柜",
		Description: "在" + sameNamePlace + "做买卖的掌柜",
	}
	scene := character.Scene{NodeID: "n1", Location: sameNamePlace, WorldTime: "第1日"}

	alpha := strings.Join(characterLoreFor(workspace, alphaScope, profile, scene, 0), "\n")
	beta := strings.Join(characterLoreFor(workspace, betaScope, profile, scene, 0), "\n")

	// Each world's own auto-loaded entry is what the character may speak from.
	if !strings.Contains(alpha, "河港最大的粮商") {
		t.Fatalf("the alpha character could not read their own world's lore:\n%s", alpha)
	}
	if !strings.Contains(beta, "矿主雇来的管事") {
		t.Fatalf("the beta character could not read their own world's lore:\n%s", beta)
	}
	// And neither may see the other's contradictory version of the same name.
	if strings.Contains(alpha, "矿主雇来的管事") || strings.Contains(alpha, "内陆矿镇") {
		t.Fatalf("the alpha character read the beta world's lore:\n%s", alpha)
	}
	if strings.Contains(beta, "河港最大的粮商") || strings.Contains(beta, "河港") {
		t.Fatalf("the beta character read the alpha world's lore:\n%s", beta)
	}

	// Resident lore is delivered as the story's own stable context, so the
	// character block must not repeat it — that would spend the budget twice and
	// would also be the channel a foreign world's resident setting leaked
	// through. Both towns are resident in their own world, so neither may appear.
	if strings.Contains(alpha, "河港，靠水运为生") || strings.Contains(beta, "内陆矿镇，以采银为生") {
		t.Fatalf("resident lore leaked into a character's own context:\nalpha=%s\nbeta=%s", alpha, beta)
	}
}

// TestCharacterLoreWithoutAScopeSeesNoWorldsBook is the packless half of the
// same rule. A story that belongs to no pack reads the shared book only, so a
// manually created story must not be handed a world's resident material merely
// because the names happen to match.
func TestCharacterLoreWithoutAScopeSeesNoWorldsBook(t *testing.T) {
	workspace, alphaScope, _ := seedTwoConflictingWorlds(t)

	// A user-authored shared entry with the same name is the one thing a packless
	// story may read.
	seeder := lore.NewStore(workspace)
	if _, err := seeder.Create(lore.ItemInput{
		Scope: lore.ScopeShared, ID: "user-zhou", Type: "setting",
		Name: sameNameActor, Content: "用户自己写的周掌柜：街口修鞋的。", LoadMode: lore.LoadModeAuto,
	}); err != nil {
		t.Fatal(err)
	}

	profile := character.Profile{ID: "zhou", StoryID: "st", Name: sameNameActor, Role: "掌柜"}
	scene := character.Scene{NodeID: "n1", WorldTime: "第1日"}

	shared := strings.Join(characterLoreFor(workspace, "", profile, scene, 0), "\n")
	if !strings.Contains(shared, "街口修鞋的") {
		t.Fatalf("a packless story could not read the user's own entry:\n%s", shared)
	}
	for _, foreign := range []string{"河港最大的粮商", "矿主雇来的管事"} {
		if strings.Contains(shared, foreign) {
			t.Fatalf("a packless story read a pack's entry (%s):\n%s", foreign, shared)
		}
	}
	_ = alphaScope
}

// TestSameNameInOneScopeIsRejectedButAllowedAcrossScopes pins the write side of
// the same-name rule: two worlds may each define 青石镇, but one world may not
// define it twice. Without the second half a pack could shadow its own entry
// and a reader would silently get whichever copy sorted first.
func TestSameNameInOneScopeIsRejectedButAllowedAcrossScopes(t *testing.T) {
	workspace, alphaScope, betaScope := seedTwoConflictingWorlds(t)
	seeder := lore.NewStore(workspace)

	if _, err := seeder.Create(lore.ItemInput{
		Scope: alphaScope, ID: "alpha-town-2", Type: "setting",
		Name: sameNamePlace, Content: "同一个世界的第二个青石镇。",
	}); err == nil {
		t.Fatal("one scope accepted two entries with the same name")
	}
	// The beta world already holds the name too, which is what makes the check
	// above meaningful rather than a global uniqueness rule in disguise.
	items, err := lore.NewScopedStore(workspace, betaScope).List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.Name == sameNamePlace {
			found = true
		}
	}
	if !found {
		t.Fatal("the beta world's same-named entry is missing, so the collision was never real")
	}
}
