package character

import (
	"strings"
	"testing"
)

// castTestProfile builds a person whose card is a predictable size, so a test
// can say how many of them fit in a budget without guessing.
func castTestProfile(id string) Profile {
	return Profile{
		ID: id, StoryID: "st", Name: id, Revision: 1,
		Description: strings.Repeat("经年累月地守着这一处地方，认得每一个常来常往的面孔。", 8),
	}
}

// TestCastBlockStaysWithinItsBudget is the capacity half of the defect the
// design names: 同等相关时按ID截断，可能让后段人物长期不可见. The trim path wrote a
// member and only then asked whether the block was over budget, so the answer
// was always one whole card too late and the block it returned was larger than
// the limit it was given. A caller that sized the rest of its context around
// this number was then over its own budget.
func TestCastBlockStaysWithinItsBudget(t *testing.T) {
	profiles := make([]Profile, 0, 6)
	for _, id := range []string{"p1", "p2", "p3", "p4", "p5", "p6"} {
		profiles = append(profiles, castTestProfile(id))
	}
	state := State{Profiles: profiles}
	const budget = 2000

	cast := CastMarkdown(StoryCast(state, Scene{}, ""), nil, budget)
	if cast == "" {
		t.Fatal("the cast block came back empty although people were registered")
	}
	if got := len([]byte(cast)); got > budget {
		t.Fatalf("the cast block is %d bytes against a %d-byte budget:\n%s", got, budget, cast)
	}
	// Trimming must not drop the whole roster: the IDs are what the turn writes
	// facts against, so at least one person has to survive.
	if !strings.Contains(cast, "(id: p1)") {
		t.Fatalf("trimming dropped every character id:\n%s", cast)
	}
}

// TestCastRosterKeepsThePeopleWithBusiness is the ordering half. The roster was
// sorted by presence and then by id, which is the same relevance for everyone
// else — so once the block had to be cut, the people who disappeared were the
// ones whose ids sorted late, however much unfinished business they carried.
// A promise still in force belongs to the turn even when that person happens not
// to be standing in the room.
func TestCastRosterKeepsThePeopleWithBusiness(t *testing.T) {
	state := State{
		Profiles: []Profile{
			castTestProfile("aaa_bystander"),
			castTestProfile("bbb_owes"),
			castTestProfile("zzz_present"),
		},
		Matters: []Matter{{
			ID: "mat_owed", StoryID: "st", BranchID: "b1", CharacterID: "bbb_owes",
			Kind: MatterPromise, Status: MatterOpen,
			Statement: "答应了把东西捎到城外。", WorldTime: "第2日",
		}},
	}
	scene := Scene{WorldTime: "第3日", Present: []string{"zzz_present"}}

	members := StoryCast(state, scene, scene.WorldTime)
	if len(members) != 3 {
		t.Fatalf("the roster lost someone before any trimming: %d members", len(members))
	}
	if members[0].Profile.ID != "zzz_present" {
		t.Fatalf("the roster does not lead with the person in the scene: %s", members[0].Profile.ID)
	}
	if members[1].Profile.ID != "bbb_owes" {
		t.Fatalf("the roster does not put the open promise ahead of a bystander: %s", members[1].Profile.ID)
	}
	if members[2].Profile.ID != "aaa_bystander" {
		t.Fatalf("a bystander outranks someone with unfinished business: %s", members[2].Profile.ID)
	}
}

// TestCastTrimKeepsTheOpenThreadNotTheAlphabet checks the two halves together
// through the rendered block: with a budget that fits only two cards, the person
// who owes an answer keeps their card and the bystander whose id sorts first
// loses theirs.
func TestCastTrimKeepsTheOpenThreadNotTheAlphabet(t *testing.T) {
	state := State{
		Profiles: []Profile{
			castTestProfile("aaa_bystander"),
			castTestProfile("bbb_owes"),
			castTestProfile("zzz_present"),
		},
		Matters: []Matter{{
			ID: "mat_owed", StoryID: "st", BranchID: "b1", CharacterID: "bbb_owes",
			Kind: MatterPromise, Status: MatterOpen,
			Statement: "答应了把东西捎到城外。", WorldTime: "第2日",
		}},
	}
	scene := Scene{WorldTime: "第3日", Present: []string{"zzz_present"}}
	members := StoryCast(state, scene, scene.WorldTime)
	// A budget between the two-card and three-card sizes: enough for the person in
	// the scene and the one who owes an answer, not enough for the bystander.
	two := len([]byte(CastMarkdown(members[:2], nil, 1<<20)))
	three := len([]byte(CastMarkdown(members, nil, 1<<20)))
	if three <= two {
		t.Fatal("the fixture has no third card to trim")
	}
	budget := two + (three-two)/2

	cast := CastMarkdown(members, nil, budget)
	if len([]byte(cast)) > budget {
		t.Fatalf("the trimmed block is %d bytes against a %d-byte budget", len([]byte(cast)), budget)
	}
	if !strings.Contains(cast, "(id: bbb_owes)") {
		t.Fatalf("trimming dropped the person who owes an answer:\n%s", cast)
	}
	// The bystander keeps an identity line but loses the full card: 降温 means less
	// room, not a person the narrator may not name.
	if strings.Contains(cast, "### aaa_bystander (id: aaa_bystander)") {
		t.Fatalf("trimming kept the alphabetically-first bystander's full card:\n%s", cast)
	}
	if !strings.Contains(cast, "(id: aaa_bystander)") {
		t.Fatalf("the trimmed block erased the bystander instead of condensing them:\n%s", cast)
	}
}

// TestCastStatesGenderBesideTheVoice is the main-story half of 同一份稳定人物资料.
// The narrator is told who the person is, and in the same breath told not to
// treat it as a temperament — the design forbids writing every man one way and
// every woman another, and the cast card is where that temptation would live.
func TestCastStatesGenderBesideTheVoice(t *testing.T) {
	profile := castTestProfile("shopkeeper")
	profile.Gender = GenderFemale
	profile.SpeechStyle = "说话慢，先问用途再报价"
	cast := CastMarkdown(StoryCast(State{Profiles: []Profile{profile}}, Scene{}, ""), nil, 0)
	if !strings.Contains(cast, "- Gender: female") {
		t.Fatalf("the cast card does not state the settled gender:\n%s", cast)
	}
	if !strings.Contains(cast, "not a stereotype") {
		t.Fatalf("the cast card states a gender with no warning against stereotyping:\n%s", cast)
	}
	if !strings.Contains(cast, "先问用途再报价") {
		t.Fatalf("the voice is missing from the same card:\n%s", cast)
	}

	unsettled := castTestProfile("stranger")
	plain := CastMarkdown(StoryCast(State{Profiles: []Profile{unsettled}}, Scene{}, ""), nil, 0)
	if strings.Contains(plain, "Gender") {
		t.Fatalf("an unsettled gender still produced a gender line:\n%s", plain)
	}
}

func TestCastShowsFixedSpeciesIdentityAndMethod(t *testing.T) {
	profile := castTestProfile("gray")
	profile.Lifeform = LifeformSpiritBeast
	profile.CultivationMethodID = "method.spirit_beast_breathing"
	cast := CastMarkdown(StoryCast(State{Profiles: []Profile{profile}}, Scene{}, ""), nil, 0)
	if !strings.Contains(cast, "Registered lifeform: spirit_beast") || !strings.Contains(cast, "method.spirit_beast_breathing") {
		t.Fatalf("the established cast context omitted fixed species qualification:\n%s", cast)
	}
}
