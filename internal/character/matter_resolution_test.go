package character

import (
	"strings"
	"testing"
)

// A model is shown matters as "[kind] statement" and, until this was fixed, was
// never shown a matter id — while the character_facts schema asked it for "the
// id of an earlier open matter". So it wrote the sentence it meant instead:
//
//	"settled_by": "阿禾以为沈青已经把她和药仓的钥匙牵扯进来，因此既怕又不敢开口。"
//
// That prose was persisted. Because the journal is replayed on every read, the
// link was then re-resolved by ApplyMatters on every later reduction, and the
// old code returned an error for a target it could not find. The result was a
// story that could no longer be opened at all: branch creation and every window
// read failed with "matter ... resolves unknown matter ...".
//
// The tests below are the two halves of the fix. One bad reference must never
// make a journal unreducible, and a reference that is obviously prose must be
// rejected while it is still a draft, where the repair pass can act on it.

// TestUnhonourableMatterLinkDoesNotBrickTheJournal is the first half. A link
// naming a matter that is not on record is dropped, not fatal: the link joins
// two matters, so dropping it leaves both intact, whereas refusing the merge
// makes every future read of the story fail.
//
// The shape modelled here is exactly how replay hits the defect. Each journal
// record's matters are reduced in turn, so the record carrying the prose arrives
// as a *projected* matter of its own reduction — which is the moment the old
// code returned the error and the story became unopenable.
func TestUnhonourableMatterLinkDoesNotBrickTheJournal(t *testing.T) {
	// The open thread the turn meant to discharge.
	existing := []Matter{{
		ID: "mat_a", StoryID: "st", BranchID: "b1", CharacterID: "ahe",
		Kind: MatterMisunderstanding, Status: MatterOpen,
		Statement: "阿禾对沈青存着防备。", WorldTime: "第2日",
	}}
	// The persisted record: settled, with prose where an id belonged.
	projected := []Matter{{
		ID: "mat_b", StoryID: "st", BranchID: "b1", CharacterID: "ahe",
		Kind: MatterMisunderstanding, Status: MatterSettled,
		Statement: "阿禾对沈青的防备松了一层。",
		SettledBy: "阿禾以为沈青已经把她和药仓的钥匙牵扯进来，因此既怕又不敢开口。",
		WorldTime: "第3日",
	}}

	merged := ApplyMatters(existing, projected)
	if len(merged) != 2 {
		t.Fatalf("a bad link changed how many matters were recorded: %d, want 2", len(merged))
	}

	// The record that carried the prose keeps its own status and statement — it
	// is a real fact about what happened — but the link that cannot be honoured
	// is gone rather than kept dangling.
	carrier, ok := MatterByID(merged, "mat_b")
	if !ok {
		t.Fatal("the record carrying the bad link was lost")
	}
	if carrier.SettledBy != "" {
		t.Fatalf("an unhonourable link survived reduction: %q", carrier.SettledBy)
	}
	if carrier.Statement != "阿禾对沈青的防备松了一层。" || carrier.Status != MatterSettled {
		t.Fatalf("dropping a bad link rewrote the record it was on: %+v", carrier)
	}

	// The thread it named stays open, which is the honest outcome: nothing on
	// record actually discharged it.
	target, ok := MatterByID(merged, "mat_a")
	if !ok {
		t.Fatal("the earlier matter was lost while dropping a bad link")
	}
	if target.Status != MatterOpen {
		t.Fatalf("a link that resolved nothing still closed the thread: %+v", target)
	}

	// And reduction is repeatable: a journal is replayed on every read, so the
	// second pass must produce the same answer as the first.
	again := ApplyMatters(merged, projected)
	if len(again) != len(merged) {
		t.Fatalf("replaying the same records changed the result: %d -> %d", len(merged), len(again))
	}
	for i := range again {
		if again[i].SettledBy != merged[i].SettledBy || again[i].Status != merged[i].Status {
			t.Fatalf("replay was not idempotent at %d: %+v vs %+v", i, merged[i], again[i])
		}
	}
}

// TestHonourableLinkStillResolvesAfterTheTolerantPath pins that the tolerance
// did not turn into indifference. A reference that does name a recorded matter
// must still settle it, or the fix would have traded a bricked journal for a
// world where nothing ever resolves.
//
// It also shows the two layers composing: a reference that is id-shaped passes
// projection even though it names nothing, so the merge is still the thing that
// has to survive it.
func TestHonourableLinkStillResolvesAfterTheTolerantPath(t *testing.T) {
	ctx := testProjectionContext()
	existing := []Matter{{
		ID: "mat_a", StoryID: "story-1", BranchID: "branch-1", CharacterID: "steward",
		Kind: MatterPromise, Status: MatterOpen,
		Statement: "管事答应替主角打听城门值守。", WorldTime: "第2日",
	}}

	// A good link and a well-formed but unknown one in the same turn.
	projected, err := Project(ctx, TurnCharacterFacts{
		Matters: []MatterDraft{
			{
				CharacterID: "steward", Kind: MatterAction, Status: MatterSettled,
				Statement: "管事回报了换班时辰。", Resolves: "mat_a",
			},
			{
				CharacterID: "steward", Kind: MatterAction, Status: MatterSettled,
				Statement: "管事提了一句别的。", Resolves: "mat_missing",
			},
		},
	})
	if err != nil {
		t.Fatalf("a well-formed reference was rejected at projection: %v", err)
	}

	merged := ApplyMatters(existing, projected.Matters)
	promise, ok := MatterByID(merged, "mat_a")
	if !ok {
		t.Fatal("the promise disappeared")
	}
	if promise.Status != MatterSettled {
		t.Fatalf("a good link stopped working once a bad one was tolerated: %+v", promise)
	}
	if promise.SettledBy != projected.Matters[0].ID {
		t.Fatalf("the promise was settled by the wrong record: %+v", promise)
	}
	if len(OpenMatters("steward", merged)) != 0 {
		t.Fatalf("the discharged promise is still open: %+v", OpenMatters("steward", merged))
	}
}

// TestProjectRejectsProseInResolves is the second half, and the one that stops
// the defect recurring. A reference that is not shaped like an id is rejected at
// projection time. That rejection surfaces as a submission diagnostic, which
// forces a repair pass, so the model is told what it got wrong instead of the
// bad value being written to the journal and discovered later.
func TestProjectRejectsProseInResolves(t *testing.T) {
	ctx := testProjectionContext()
	_, err := Project(ctx, TurnCharacterFacts{
		Matters: []MatterDraft{{
			CharacterID: "steward", Kind: MatterAction, Status: MatterSettled,
			Statement: "管事把换班时辰说了。",
			Resolves:  "管事以为主角已经知道这件事，所以没有细说。",
		}},
	})
	if err == nil {
		t.Fatal("a sentence in resolves was accepted as a matter reference")
	}
	if !strings.Contains(err.Error(), "not a matter id") {
		t.Fatalf("the rejection does not say what was wrong: %v", err)
	}

	// The same rule applies to a well-formed but empty-ish reference.
	if _, err := Project(ctx, TurnCharacterFacts{
		Matters: []MatterDraft{{
			CharacterID: "steward", Kind: MatterAction, Status: MatterSettled,
			Statement: "管事把换班时辰说了。", Resolves: "mat_a mat_b",
		}},
	}); err == nil {
		t.Fatal("a space-separated reference was accepted as one matter id")
	}

	// A real id still passes, so the check constrains prose and not the field.
	projected, err := Project(ctx, TurnCharacterFacts{
		Matters: []MatterDraft{{
			CharacterID: "steward", Kind: MatterAction, Status: MatterSettled,
			Statement: "管事把换班时辰说了。", Resolves: "mat_dm4td9e5s300eb246a9e",
		}},
	})
	if err != nil {
		t.Fatalf("a well-formed reference was rejected: %v", err)
	}
	if projected.Matters[0].SettledBy != "mat_dm4td9e5s300eb246a9e" {
		t.Fatalf("the reference did not survive projection: %+v", projected.Matters[0])
	}
}

// TestMatterIDsAreShownWhereMattersAreRendered is the root cause rather than the
// symptom. The field asks for an id; the fix is only complete if the id is
// actually in front of the model, or the next generation invents prose again.
func TestMatterIDsAreShownWhereMattersAreRendered(t *testing.T) {
	state := State{
		Profiles: []Profile{{
			ID: "guardsman", StoryID: "st", Name: "守夜人", Revision: 1,
		}},
		Matters: []Matter{{
			ID: "mat_a", StoryID: "st", BranchID: "b1", CharacterID: "guardsman",
			Kind: MatterPermission, Status: MatterOpen,
			Statement: "守夜人允许主角今夜从北门入城。", WorldTime: "第3日",
		}},
		Knowledge: []KnowledgeRecord{{
			ID: "knw_a", CharacterID: "guardsman", Statement: "北门今夜换防。",
			Nature: NatureExperienced, Audience: AudienceSingle, WorldTime: "第3日",
		}},
	}
	profile := Profile{ID: "guardsman", StoryID: "st", Name: "守夜人", Revision: 1}
	scene := Scene{NodeID: "north-gate", WorldTime: "第3日"}

	cast := CastMarkdown(StoryCast(state, scene, scene.WorldTime), nil, 0)
	if !strings.Contains(cast, "mat_a") {
		t.Fatalf("the cast block lists a matter without its id, so resolves cannot be filled:\n%s", cast)
	}

	prepared := Prepare(PrepareInput{
		StoryID: "st", Profile: profile,
		Revision: IdentityRevision{BranchID: "b1", ProfileRevision: 1},
		Scene:    scene, Matters: state.Matters, Knowledge: state.Knowledge,
	})
	if !strings.Contains(prepared.Interactions, "mat_a") {
		t.Fatalf("the prepared interactions list a matter without its id:\n%s", prepared.Interactions)
	}
	if !strings.Contains(prepared.Memory, "knw_a") {
		t.Fatalf("the prepared memory lists a belief without its id:\n%s", prepared.Memory)
	}
}

// TestOpenMatterPointsAtTheMessageChannel is the other half of the message
// defect. A character who owes word and cannot appear in person is exactly the
// case character_facts.messages exists for, but the narrator's cast block listed
// the promise without ever connecting it to the message channel. In the
// acceptance run an appointment came due, the clock passed sunset, and the turn
// recorded the promise as abandoned with nothing sent — 82 character records
// across every journal, none carrying a message.
//
// The hook is attached to every open thread rather than only to dated ones,
// because the model routinely leaves the deadline field empty even when the
// prose named a time: the appointment that failed in the acceptance run was
// recorded with no deadline at all. The schema's rule against messaging every
// turn is restated in the same breath, so naming the channel does not become
// permission to use it.
func TestOpenMatterPointsAtTheMessageChannel(t *testing.T) {
	profile := Profile{ID: "hejing", StoryID: "st", Name: "贺敬", Revision: 1}
	dated := Matter{
		ID: "mat_due", StoryID: "st", BranchID: "b1", CharacterID: "hejing",
		Kind: MatterPromise, Status: MatterOpen,
		Statement: "贺敬答应日落前把名字捎到镇西口外的破棚子。",
		Deadline:  "第3日 日落之前", WorldTime: "第2日 午后",
	}
	undated := Matter{
		ID: "mat_open", StoryID: "st", BranchID: "b1", CharacterID: "hejing",
		Kind: MatterPromise, Status: MatterOpen,
		Statement: "贺敬答应改日再谈。", WorldTime: "第2日 午后",
	}
	settled := Matter{
		ID: "mat_done", StoryID: "st", BranchID: "b1", CharacterID: "hejing",
		Kind: MatterPromise, Status: MatterSettled,
		Statement: "贺敬答应的事已经办了。", WorldTime: "第2日 午后",
	}
	state := State{Profiles: []Profile{profile}, Matters: []Matter{dated, undated, settled}}
	scene := Scene{WorldTime: "第2日 午后", Present: []string{"hejing"}}

	cast := CastMarkdown(StoryCast(state, scene, scene.WorldTime), nil, 0)
	// Both open threads name the channel, including the undated one.
	if got := strings.Count(cast, "character_facts.messages"); got != 2 {
		t.Fatalf("the message hook appears %d times, want once per open thread (2):\n%s", got, cast)
	}
	if !strings.Contains(cast, "第3日 日落之前") {
		t.Fatalf("the dated thread's own deadline is missing:\n%s", cast)
	}
	// A settled thread is not unfinished work, so it stays out of the open list
	// and never attracts a message. It is still reported — §5.5 后果必须有消费者 —
	// but in its own section, which is what keeps the two readings apart.
	unfinished := cast
	if start := strings.Index(unfinished, "- Unfinished with the protagonist:"); start >= 0 {
		unfinished = unfinished[start:]
		if end := strings.Index(unfinished, "- Already settled with the protagonist"); end >= 0 {
			unfinished = unfinished[:end]
		}
	}
	if strings.Contains(unfinished, "mat_done") {
		t.Fatalf("a settled thread was listed as unfinished:\n%s", cast)
	}
	resolved := cast
	if start := strings.Index(resolved, "- Already settled with the protagonist"); start >= 0 {
		resolved = resolved[start:]
	} else {
		t.Fatalf("a settled thread is not reported at all, so nobody can acknowledge it:\n%s", cast)
	}
	if !strings.Contains(resolved, "mat_done") {
		t.Fatalf("the settled thread is missing from the resolved section:\n%s", cast)
	}
	if strings.Contains(resolved, "character_facts.messages") {
		t.Fatalf("a settled thread still attracts the message hook:\n%s", cast)
	}
	// Naming the channel must not read as a licence to message every turn.
	if !strings.Contains(cast, "Do not write merely because a thread is open") {
		t.Fatalf("the hook does not restate the no-per-turn rule:\n%s", cast)
	}
}

// TestUnorderableWorldTimeIsNotExpiry reproduces the defect that made the
// acceptance run produce no messages at all.
//
// A story authors its own world-time labels, and this one used labels with no
// digits in them: the promise was created at "测灵台开张当日，入夜" and lapsed at
// "今日日落前". The comparison used to fall back to string order, so 测 (U+6D4B)
// sorted past 今 (U+4ECA) and the promise was filed as already lapsed the moment
// it was created. The cast block shows a lapsed thread under a heading that
// carries no message hook, so the one place the character had a concrete reason
// to write was never reachable — 21 character records across the journal, not one
// carrying a message.
//
// Every existing lapse test used numeric labels ("第2日" / "第3日 日落之前"), so
// the fallback branch had no coverage and the Go suite stayed green while the
// product produced nothing. This pins the branch that was missing: a pair with no
// shared scale is unordered, and an unknown order must not be read as expiry.
func TestUnorderableWorldTimeIsNotExpiry(t *testing.T) {
	// The two labels from the acceptance story, in the order that failed.
	const created = "测灵台开张当日，入夜"
	const deadline = "今日日落前"
	if got := compareWorldTime(created, deadline); got != 0 {
		t.Fatalf("two labels with no digits compared as %d, want 0 (unordered); "+
			"a nonzero result is what expired every such thread on creation", got)
	}
	// Reversing the arguments must not turn an unknown order into a known one.
	if got := compareWorldTime(deadline, created); got != 0 {
		t.Fatalf("the reversed pair compared as %d, want 0", got)
	}

	profile := Profile{ID: "futan", StoryID: "st", Name: "符摊女修", Revision: 1}
	appointment := Matter{
		ID: "mat_due", StoryID: "st", BranchID: "b1", CharacterID: "futan",
		Kind: MatterPromise, Status: MatterOpen,
		Statement: "沈青与葛老汉说定三十文的样货买卖，约定日落前捎话到西口外破棚子。",
		Deadline:  deadline, WorldTime: created,
	}
	state := State{Profiles: []Profile{profile}, Matters: []Matter{appointment}}
	scene := Scene{WorldTime: created, Present: []string{"futan"}}

	if appointment.Lapsed(scene.WorldTime) {
		t.Fatalf("a thread created at its own world time was already lapsed; " +
			"this is the bug: the message hook lives only under the open heading")
	}

	cast := CastMarkdown(StoryCast(state, scene, scene.WorldTime), nil, 0)
	if !strings.Contains(cast, "Unfinished with the protagonist") {
		t.Fatalf("the thread was not shown as unfinished, so the message channel "+
			"was never offered to the model:\n%s", cast)
	}
	if !strings.Contains(cast, "character_facts.messages") {
		t.Fatalf("the open thread did not name the message channel:\n%s", cast)
	}
	if strings.Contains(cast, "Lapsed with the protagonist") {
		t.Fatalf("the thread was filed as lapsed at its own creation time:\n%s", cast)
	}

	// Numeric labels must keep ordering exactly as before; the fix is confined to
	// the unorderable branch.
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"第2日 午后", "第3日 日落之前", -1},
		{"第3日 日落之前", "第2日 午后", 1},
		{"第3日 日落之前", "第3日 日落之前", 0},
		// A label that mixes a number with an unorderable one is still unorderable:
		// there is no shared scale to compare 3 against "今日".
		{"第3日", "今日日落前", 0},
	} {
		if got := compareWorldTime(c.a, c.b); got != c.want {
			t.Fatalf("compareWorldTime(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestLooksLikeMatterID pins the boundary. The check exists to catch a sentence
// in an id field, so it must reject prose and punctuation while leaving every
// shape an id actually takes alone.
func TestLooksLikeMatterID(t *testing.T) {
	for _, ok := range []string{"mat_a", "mat_dm4td9e5s300eb246a9e", "mat-1", "mat.1", "knw:x"} {
		if !looksLikeMatterID(ok) {
			t.Fatalf("a well-formed id was rejected: %q", ok)
		}
	}
	for _, bad := range []string{
		"",
		"阿禾以为沈青已经把她牵扯进来。",
		"mat_a mat_b",
		"the earlier promise",
		"mat_a，",
		strings.Repeat("a", 65),
	} {
		if looksLikeMatterID(bad) {
			t.Fatalf("prose was accepted as an id: %q", bad)
		}
	}
}
