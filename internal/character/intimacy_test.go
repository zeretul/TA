package character

import "testing"

func adultActivity(phase IntimacyPhase) IntimacyActivity {
	return IntimacyActivity{
		ID: "int_1", StoryID: "st", BranchID: "main", Phase: phase,
		Participants: []IntimacyParticipant{
			{CharacterID: "a", Status: IntimacyAccepted, Adult: true},
			{CharacterID: "b", Status: IntimacyPending, Adult: true},
		},
		GroupID: "g1",
		Version: 1,
	}.Normalize()
}

// TestIntimacyConditionalIsNotConsent is the design's DUAL14 rule at the record
// level: a person who agrees only if something is settled is still negotiating,
// so a roster built from ConsentedRoster must not contain them.
func TestIntimacyConditionalIsNotConsent(t *testing.T) {
	activity := adultActivity(IntimacyInviting)
	activity = activity.WithResponse("b", IntimacyConditional, "先把手头的事办完", "第2日")
	if roster := activity.ConsentedRoster(); len(roster) != 1 || roster[0] != "a" {
		t.Fatalf("ConsentedRoster = %v, want only the unqualified yes", roster)
	}
	if activity.Phase != IntimacyReady {
		t.Fatalf("phase = %q, want ready once somebody actually accepted", activity.Phase)
	}
	if _, err := activity.Confirm("g1", "第2日", 0); err != nil {
		t.Fatalf("confirm with one real acceptance: %v", err)
	}
}

// TestIntimacyFullRefusalEndsWithoutARoom is §3.3's "若所有人拒绝则不能开始".
// The activity must be writable as ended even though nobody consented and no
// conversation space was ever made.
func TestIntimacyFullRefusalEndsWithoutARoom(t *testing.T) {
	activity := IntimacyActivity{
		ID: "int_1", StoryID: "st", BranchID: "main", Phase: IntimacyInviting,
		Participants: []IntimacyParticipant{
			{CharacterID: "a", Status: IntimacyPending, Adult: true},
		},
		Version: 1,
	}.Normalize()
	activity = activity.WithResponse("a", IntimacyDeclined, "", "第2日")
	if activity.Phase != IntimacyEnded {
		t.Fatalf("phase = %q, want ended when everyone declined", activity.Phase)
	}
	if err := activity.Validate(); err != nil {
		t.Fatalf("a full refusal must be a valid record: %v", err)
	}
}

// TestIntimacyNonAdultCannotBeAParticipant is §3.1's age rule: the record itself
// refuses a person whose own material did not establish an adult age, so no path
// can produce one by skipping the entry check.
func TestIntimacyNonAdultCannotBeAParticipant(t *testing.T) {
	activity := adultActivity(IntimacyInviting)
	if _, err := activity.AddParticipant("c", false, "第2日"); err == nil {
		t.Fatal("inviting someone without an established adult age must be refused")
	}
	activity.Participants = append(activity.Participants, IntimacyParticipant{CharacterID: "c", Status: IntimacyPending})
	if err := activity.Validate(); err == nil {
		t.Fatal("an activity carrying a non-adult participant must not validate")
	}
}

// TestIntimacyAnswerMidActivityKeepsItRunning is §13.1's mid-activity rule at the
// record level. A person's answer changes the roster, not the phase: recomputing
// would drop a running room back to "ready" and let a second confirmation start
// it again.
func TestIntimacyAnswerMidActivityKeepsItRunning(t *testing.T) {
	activity := adultActivity(IntimacyActive)
	next := activity.WithResponse("b", IntimacyAccepted, "", "第3日")
	if next.Phase != IntimacyActive {
		t.Fatalf("phase = %q, want the activity to stay active", next.Phase)
	}
	if _, err := next.Confirm("g1", "第3日", 0); err == nil {
		t.Fatal("a running activity must not accept a second confirmation")
	}
	if roster := next.ConsentedRoster(); len(roster) != 2 {
		t.Fatalf("ConsentedRoster = %v, want both people once the second agreed", roster)
	}
}

// TestIntimacyAddParticipantMidActivityKeepsItRunning guards the other half: an
// invitation sent into a running activity does not restart it either.
func TestIntimacyAddParticipantMidActivityKeepsItRunning(t *testing.T) {
	activity := adultActivity(IntimacyActive)
	next, err := activity.AddParticipant("c", true, "第3日")
	if err != nil {
		t.Fatalf("inviting someone into a running activity: %v", err)
	}
	if next.Phase != IntimacyActive {
		t.Fatalf("phase = %q, want active", next.Phase)
	}
	if participant, ok := next.Participant("c"); !ok || participant.Status != IntimacyPending {
		t.Fatalf("new invitee = %+v, want pending and not consented", participant)
	}
	if roster := next.ConsentedRoster(); ContainsID(roster, "c") {
		t.Fatalf("ConsentedRoster = %v, want the new invitee left out until they answer", roster)
	}
}

// TestIntimacyStaleConfirmationIsRefused is §3.3's version binding: a
// confirmation based on a roster the player was never shown must not be applied.
func TestIntimacyStaleConfirmationIsRefused(t *testing.T) {
	activity := adultActivity(IntimacyReady)
	if _, err := activity.Confirm("g1", "第3日", activity.Version+1); err == nil {
		t.Fatal("a stale confirmation must be refused")
	}
}

// TestIntimacySpaceIDsKeepTheRoomApart is the audience guarantee: a person asked
// to join a running activity has an invitation space of their own, and it is not
// the room.
func TestIntimacySpaceIDsKeepTheRoomApart(t *testing.T) {
	activity := adultActivity(IntimacyActive)
	if activity.SpaceID() != "g1" {
		t.Fatalf("SpaceID = %q, want the confirmed room", activity.SpaceID())
	}
	if invite := activity.InvitationSpaceID("c"); invite == activity.SpaceID() {
		t.Fatalf("invitation space = %q, must not be the running room", invite)
	}
	unsent := IntimacyActivity{ID: "int_2", StoryID: "st", BranchID: "main"}.Normalize()
	if unsent.SpaceID() != "int_2:space" {
		t.Fatalf("an unstarted activity's space = %q, want its invitation parley", unsent.SpaceID())
	}
}

// TestIntimacyGroupAudienceExcludesNonConsentingMembers ties the record to the
// audience rule. A group's audience is its active members, so a person who is not
// in the room's membership cannot hear what is said there.
func TestIntimacyGroupAudienceExcludesNonConsentingMembers(t *testing.T) {
	group := Group{ID: "g1", StoryID: "st", BranchID: "main", Type: GroupOther, Name: "双修"}.
		Join("a", "第2日").Join("b", "第2日")
	audience := group.AudienceFor("a", []string{"a", "b", "c"})
	if ContainsID(audience, "c") {
		t.Fatalf("audience = %v, want the bystander left out", audience)
	}
	afterBLeft := group.Leave("b", "第3日").AudienceFor("a", []string{"a", "b", "c"})
	if ContainsID(afterBLeft, "b") {
		t.Fatalf("audience after b left = %v, want b left out", afterBLeft)
	}
}
