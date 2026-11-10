package quest

// 自制-并行事项与到期事件 §五 keeps an agreed deadline a fact about the world
// rather than about the player's attention: it runs on committed formal time,
// it closes the opportunity rather than erasing what was done, and it is stamped
// once so two branches cannot disagree about when one promise ended.

import "testing"

func deadlineDefinition(seconds int64) Definition {
	definition := Definition{
		SchemaVersion: SchemaVersion,
		ID:            "quest.deadline_fixture",
		Version:       "1.0",
		PackageID:     "fanren-jie", PackageVersion: "3",
		Kind:  KindErrand,
		Title: "有期限的差事",
		Brief: "一件在约定时间内交付的差事。",
		DeadlineSeconds: seconds,
		Objectives: []Objective{{
			ID: "quest.deadline_fixture.done", Description: "办完这件事。",
			Evidence: []Evidence{{Kind: EvidenceRealmReward, RefID: "realm.herb_ref"}},
		}},
		Rewards: []Reward{{Kind: RewardCurrency, RefID: "灵石", Quantity: 5}},
	}
	return definition
}

func acceptedWithDeadline(definition Definition, acceptedAt int64) Instance {
	instance := Instance{TaskID: definition.ID, TaskVersion: definition.Version, Status: StatusAccepted}
	return StampDeadline(definition, instance, acceptedAt)
}

// TestStampDeadlineCountsFromAcceptance keeps two acceptances of the same task
// a week apart on their own separate limits.
func TestStampDeadlineCountsFromAcceptance(t *testing.T) {
	definition := deadlineDefinition(2 * 24 * 60 * 60)
	early := acceptedWithDeadline(definition, 1000)
	late := acceptedWithDeadline(definition, 1000+7*24*60*60)
	if early.DeadlineSeconds != 1000+2*24*60*60 {
		t.Fatalf("the first acceptance runs from its own moment: %d", early.DeadlineSeconds)
	}
	if late.DeadlineSeconds != early.DeadlineSeconds+7*24*60*60 {
		t.Fatalf("the second acceptance must not inherit the first limit: %d", late.DeadlineSeconds)
	}
}

// TestNoAuthoredDeadlineStampsNothing keeps "未约定期限" an honest answer rather
// than a countdown invented from the clock.
func TestNoAuthoredDeadlineStampsNothing(t *testing.T) {
	definition := deadlineDefinition(0)
	instance := acceptedWithDeadline(definition, 5000)
	if _, has := instance.Deadline(); has {
		t.Fatalf("a task with no agreed limit must have no deadline: %#v", instance)
	}
	if instance.ExpiredAt(1 << 40) {
		t.Fatal("an instance with no deadline is never expired")
	}
}

// TestDeadlineExpiresOnlyAtOrAfterTheInstant is the boundary itself.
func TestDeadlineExpiresOnlyAtOrAfterTheInstant(t *testing.T) {
	definition := deadlineDefinition(100)
	instance := acceptedWithDeadline(definition, 0)
	if instance.ExpiredAt(99) {
		t.Fatal("the instant before the deadline must still be in time")
	}
	if !instance.ExpiredAt(100) {
		t.Fatal("the deadline instant itself is past the agreement")
	}
	next, expired := instance.Expire(99)
	if expired || next.Status != StatusAccepted {
		t.Fatal("a task inside its limit must not expire")
	}
	next, expired = instance.Expire(100)
	if !expired || next.Status != StatusExpired {
		t.Fatal("a task at its limit must expire")
	}
}

// TestACompletedTaskIsNotExpiredByLaterTime is §五's "期限前完成但还没领报酬"
// case: the work was finished in time, so a later instant does not take it away.
func TestACompletedTaskIsNotExpiredByLaterTime(t *testing.T) {
	definition := deadlineDefinition(100)
	instance := acceptedWithDeadline(definition, 0)
	instance, added := instance.ApplyFact(definition, Fact{Kind: EvidenceRealmReward, RefID: "realm.herb_ref"}, "realm.reward", 50)
	if len(added) != 1 || instance.Status != StatusCompleted {
		t.Fatalf("the fact inside the limit must complete the task: %#v", instance)
	}
	if instance.ExpiredAt(1000) {
		t.Fatal("a completed task is not expired by time passing afterwards")
	}
	if next, expired := instance.Expire(1000); expired || next.Status != StatusCompleted {
		t.Fatal("expiry must not touch a completed task")
	}
}

// TestAFactAfterTheDeadlineDoesNotTick keeps a late deed from being counted as
// an on-time one.
func TestAFactAfterTheDeadlineDoesNotTick(t *testing.T) {
	definition := deadlineDefinition(100)
	instance := acceptedWithDeadline(definition, 0)
	advanced, added := instance.ApplyFact(definition, Fact{Kind: EvidenceRealmReward, RefID: "realm.herb_ref"}, "realm.reward", 150)
	if len(added) != 0 {
		t.Fatalf("a fact after the agreed instant must not tick: %#v", added)
	}
	if advanced.Status != StatusAccepted {
		t.Fatalf("the instance must be unchanged by a late fact: %#v", advanced)
	}
}

// TestExpiryKeepsWorkAlreadyDone is the "closes the opportunity, does not erase
// what happened" rule.
func TestExpiryKeepsWorkAlreadyDone(t *testing.T) {
	definition := deadlineDefinition(100)
	instance := Instance{
		TaskID: definition.ID, TaskVersion: definition.Version, Status: StatusAccepted,
		CompletedObjectiveIDs: []string{"quest.deadline_fixture.done"},
		SourceRefs:            map[string]string{"quest.deadline_fixture.done": "realm.reward"},
	}
	instance = StampDeadline(definition, instance, 0)
	expired, ok := instance.Expire(200)
	if !ok {
		t.Fatal("the task must expire")
	}
	if len(expired.CompletedObjectiveIDs) != 1 || expired.SourceRefs["quest.deadline_fixture.done"] != "realm.reward" {
		t.Fatalf("expiry must keep the work that was really done: %#v", expired)
	}
	if diagnostics := ValidateInstance(definition, expired); len(diagnostics) != 0 {
		t.Fatalf("an expired instance with real work must stay valid: %#v", diagnostics)
	}
}

// TestAnExpiredTaskCannotHavePaid keeps expiry from hiding a double payment.
func TestAnExpiredTaskCannotHavePaid(t *testing.T) {
	definition := deadlineDefinition(100)
	instance := Instance{
		TaskID: definition.ID, TaskVersion: definition.Version, Status: StatusExpired,
		CompletedObjectiveIDs: []string{"quest.deadline_fixture.done"},
		SettledRewards:        true,
	}
	instance.DeadlineSeconds = 100
	diagnostics := ValidateInstance(definition, instance)
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "settled_expired" {
			found = true
		}
	}
	if !found {
		t.Fatalf("an expired task that paid must be refused: %#v", diagnostics)
	}
}

// TestDefinitionRejectsAnUnboundedDeadline keeps an authored limit from
// overflowing the clock it is added to.
func TestDefinitionRejectsAnUnboundedDeadline(t *testing.T) {
	for _, seconds := range []int64{-1, MaxDeadlineSeconds + 1} {
		definition := deadlineDefinition(seconds)
		diagnostics := ValidateDefinition(definition)
		found := false
		for _, diagnostic := range diagnostics {
			if diagnostic.Code == "invalid_deadline" {
				found = true
			}
		}
		if !found {
			t.Fatalf("deadline %d must be refused: %#v", seconds, diagnostics)
		}
	}
	if diagnostics := ValidateDefinition(deadlineDefinition(MaxDeadlineSeconds)); len(diagnostics) != 0 {
		t.Fatalf("the authored bound itself must be allowed: %#v", diagnostics)
	}
}

// TestAnOldAcceptanceKeepsTheTermsItWasTakenUnder is the old-save rule: an
// instance stored before its task carried a deadline is not invalidated.
func TestAnOldAcceptanceKeepsTheTermsItWasTakenUnder(t *testing.T) {
	definition := deadlineDefinition(100)
	instance := Instance{TaskID: definition.ID, TaskVersion: definition.Version, Status: StatusAccepted}
	if diagnostics := ValidateInstance(definition, instance); len(diagnostics) != 0 {
		t.Fatalf("an acceptance with no stored stamp must stay valid: %#v", diagnostics)
	}
	if instance.ExpiredAt(1 << 40) {
		t.Fatal("an acceptance with no stamp has no limit to expire on")
	}
}

// TestTheAuthoredPairHasOneOpenEndAndOneDeadline is the shape the design asks
// the first batch to demonstrate: A with no limit, B with a clear agreement.
func TestTheAuthoredPairHasOneOpenEndAndOneDeadline(t *testing.T) {
	handover, ok := DefinitionByID(TaskHerbHandover)
	if !ok {
		t.Fatal("the herb handover must be registered")
	}
	if handover.DeadlineSeconds != 0 {
		t.Fatalf("the open-ended errand must stay open-ended: %d", handover.DeadlineSeconds)
	}
	materials, ok := DefinitionByID(TaskSuQianMaterials)
	if !ok {
		t.Fatal("苏浅's card must be registered")
	}
	if materials.DeadlineSeconds <= 0 {
		t.Fatalf("the personal matter must carry a real limit: %d", materials.DeadlineSeconds)
	}
	for _, definition := range Definitions() {
		if diagnostics := ValidateDefinition(definition); len(diagnostics) != 0 {
			t.Fatalf("registered task %s is invalid: %#v", definition.ID, diagnostics)
		}
	}
}
