package interactive

import "ta/internal/quest"

// This file settles what a committed interval of formal time closes.
//
// 自制-并行事项与到期事件 §五 asks the rules to process every registered matter
// whose node the interval crossed, not only the one the player was looking at.
// The interval itself is always committed by a record that carries its own
// before and after instants — a journey, a cultivation stretch, a rest — so this
// pass is called from those commits and never from a read. Opening the panel
// cannot expire anything, because nothing here runs outside a commit.
//
// It is deliberately narrow. A task is the only registered matter with an
// instant of its own today: a public event's stages are derived from the clock
// whenever the event is read, so its windows need no record to close them. When
// another matter gains a real instant, it belongs here rather than in a second
// scheduler.
//
// NPC growth rides the same pass for the same reason: it is the other matter
// whose settlement is owned by an instant rather than by a read, and paying a
// character for a stretch of time is exactly "closing what the interval
// crossed". Sharing the pass is what makes "one settlement per interval" true
// by construction rather than by each commit remembering to call it.

// storyIntervalEvents returns the records one interval closes, chained after
// parentID, together with the new head.
//
// clockBefore and clockAfter are the record's own instants, not the caller's
// estimate: a turn that named a decade but stopped at a bottleneck spent the
// part before the boundary, and the interval that closed is the part that was
// really spent.
//
// The projection is not folded here. The journal applies each payload as it
// writes it, so folding now would leave the branch holding an expiry whose
// transaction failed — a state the player would read and no record would
// support. A reroll republishes the interval's clock record at the same position
// and therefore re-emits this pass from the rewound projection, which is what
// keeps the expiries on the branch's path.
// turnID names the main-story turn the interval was spent inside, when there is
// one. It is carried onto the growth records so a reader can tell which turn
// paid for an NPC's progress; it is not part of any interval's identity.
//
// settledActorID names the character whose growth over exactly this interval the
// caller has already settled through the cultivation rules. One actor's one real
// interval has one settlement owner: a cultivation record that priced an NPC's
// thirty days is the answer for those thirty days, and letting the automatic
// pass price the same actor over the same seconds writes a second, contradictory
// figure — the later record wins and the first settlement is lost. Empty means
// nobody claimed the interval, which is the ordinary case for a rest, a walk or
// a craft: the protagonist's own time is theirs, and every NPC grows here.
func storyIntervalEvents(projection *storyBranchProjection, system StoryDirectorActorStateSystem, branchID, parentID, now, turnID, settledActorID string, clockBefore, clockAfter StoryClock) ([]any, string) {
	if projection == nil || clockAfter.Seconds <= clockBefore.Seconds {
		return nil, parentID
	}
	events := questDeadlineEvents(projection, branchID, turnID, parentID, now, clockBefore.Seconds, clockAfter.Seconds)
	head := parentID
	for _, event := range events {
		if record, ok := event.(QuestRecordEvent); ok {
			head = record.ID
		}
	}
	echoes, echoHead := questEchoEvents(projection, branchID, turnID, head, now, clockAfter.Seconds)
	events = append(events, echoes...)
	head = echoHead
	// Growth is chained after the expiries so the whole interval folds in one
	// order on every replay: a task that ran out before the actor trained is a
	// fact the actor trained without.
	growth, growthHead := npcGrowthEvents(projection, system, branchID, head, now, turnID, settledActorID, clockBefore, clockAfter)
	events = append(events, growth...)
	hazards, hazardHead := adventureOpeningEvents(projection, system, branchID, growthHead, now, turnID, clockAfter)
	events = append(events, hazards...)
	return events, hazardHead
}

// intervalRewindQuests is the task map as it stood before an interval's own
// expiries folded in.
//
// A reroll re-emits the pass from a replacement record, and by then the live
// projection already reports those tasks as expired — the journal folded the
// superseded records when they were appended. Expiring an already-expired task
// is a no-op, so the re-emission would silently produce nothing and the branch
// would lose the expiry with the record it replaced. Rewinding only the statuses
// the interval itself set, and only for tasks whose deadline the interval
// crosses, restores the position the pass is written from without resurrecting
// an expiry some earlier interval really committed.
func intervalRewindQuests(projection *storyBranchProjection, clockBefore, clockAfter StoryClock) map[string]quest.Instance {
	if projection == nil || len(projection.Quests) == 0 || clockAfter.Seconds <= clockBefore.Seconds {
		return nil
	}
	rewound := make(map[string]quest.Instance, len(projection.Quests))
	for id, instance := range projection.Quests {
		if instance.Status == quest.StatusExpired {
			deadline, has := instance.Deadline()
			if has && deadline > clockBefore.Seconds && deadline <= clockAfter.Seconds {
				// This is the expiry the interval being re-emitted wrote. Nothing
				// earlier could have set it, because the interval's start is before
				// the deadline.
				instance.Status = quest.StatusAccepted
			}
		}
		instance = cloneQuestInstance(instance)
		if e := instance.CompletionEcho; e != nil && e.Resolved && e.DueSeconds > clockBefore.Seconds && e.DueSeconds <= clockAfter.Seconds {
			e.Resolved = false
		}
		for objective, result := range instance.Choices {
			if result.EchoResolved && result.EchoDueSeconds > clockBefore.Seconds && result.EchoDueSeconds <= clockAfter.Seconds {
				result.EchoResolved = false
				instance.Choices[objective] = result
			}
		}
		rewound[id] = instance
	}
	return rewound
}
