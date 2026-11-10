package history

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"ta/internal/agents/conversationjournal"
	"testing"
	"time"
)

func campaign(t *testing.T) (*Store, State) {
	t.Helper()
	store := NewStore(t.TempDir())
	state, err := store.Create(context.Background(), CreateRequest{Name: "明末试局", CommandID: "create"})
	if err != nil {
		t.Fatal(err)
	}
	return store, state
}
func apply(t *testing.T, s *Store, state State, c Command) Receipt {
	t.Helper()
	c.ExpectedRevision = state.Revision
	r, err := s.Apply(context.Background(), state.ID, c)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func order(id, kind, target, who string, budget, grain int64, quarters int) Instruction {
	return Instruction{ID: id, Kind: kind, Target: target, MinisterID: who, Budget: budget, Grain: grain, Quarters: quarters}
}
func quarter(t *testing.T, s *Store, state State, id string) State {
	t.Helper()
	return apply(t, s, state, Command{ID: id, Kind: "advance", Quarter: &QuarterPlan{}}).State
}

func TestCompositeEdictDelayedDependencyAndIndividualOutcomes(t *testing.T) {
	store, state := campaign(t)
	relief := order("relief", "relief", "shaanxi", "bi-ziyan", 50000, 30000, 1)
	training := order("training", "training", "liaodong", "sun-chengzong", 40000, 0, 1)
	construction := order("water", "construction", "shanxi", "fan-jingwen", 200000, 0, 4)
	construction.NotBeforeTurn = 2
	construction.DependsOn = []string{"relief"}
	r := apply(t, store, state, Command{ID: "edict", Kind: "edict", Edict: &EdictPlan{Text: "赈济陕西，操练边军；第三季以后且赈济完成再兴修水利；海外使团待议。", Instructions: []Instruction{relief, training, construction, {ID: "diplomacy", Kind: "diplomacy", Reason: "first_slice_unsupported"}}}})
	if len(r.Outcomes) != 4 || r.Outcomes[3].Status != "unsupported" || len(r.State.Projects) != 3 || r.State.Treasury != state.Treasury {
		t.Fatalf("bad independent outcomes: %+v", r.Outcomes)
	}
	state = quarter(t, store, r.State, "q1")
	if state.Projects[0].Status != "completed" || state.Projects[1].Status != "completed" || state.Projects[2].Progress != 0 {
		t.Fatal("multi-order or deferred execution failed")
	}
	if state.Regions[2].Disaster >= 22 || state.Regions[1].Training <= 25 {
		t.Fatal("commands did not change real crisis/training")
	}
	state = quarter(t, store, state, "q2")
	if state.Projects[2].Progress != 0 {
		t.Fatal("not-before condition ran early")
	}
	state = quarter(t, store, state, "q3")
	if state.Projects[2].Progress != 100 {
		t.Fatalf("project did not start when due: %+v", state.Projects[2])
	}
}

func TestFourQuarterProjectColdReadAndResourceConservation(t *testing.T) {
	store, state := campaign(t)
	initialTreasury := state.Treasury
	initialGrain := state.Grain
	state = apply(t, store, state, Command{ID: "build", Kind: "edict", Edict: &EdictPlan{Text: "四季兴修水利", Instructions: []Instruction{order("water", "construction", "shaanxi", "fan-jingwen", 200000, 10000, 4)}}}).State
	income, upkeep := int64(0), int64(0)
	for i := 0; i < 4; i++ {
		state = quarter(t, store, state, fmt.Sprintf("quarter-%d", i))
		var err error
		store = NewStore(filepath.Dir(store.root))
		state, err = store.Read(context.Background(), state.ID)
		if err != nil {
			t.Fatal(err)
		}
		report := state.Reports[len(state.Reports)-1]
		income += report.Income
		upkeep += report.Upkeep
		p := state.Projects[0]
		escrow := p.Budget - p.Spent
		if state.Treasury+escrow+p.Spent != initialTreasury+income-upkeep {
			t.Fatal("silver conservation failed")
		}
		if state.Grain+(p.Grain-p.GrainSpent)+p.GrainSpent != initialGrain {
			t.Fatal("grain conservation failed")
		}
		if p.Progress != (i+1)*100 {
			t.Fatalf("wrong progress at %d: %+v", i, p)
		}
	}
	if state.Projects[0].Status != "completed" || state.Projects[0].Spent != 200000 || state.Regions[2].Irrigation != 10 || state.Year != 1628 || state.Quarter != 3 {
		t.Fatalf("bad final state: %+v", state)
	}
}

func TestPauseFundingReassignResumeAndNoAutomaticPrivySpend(t *testing.T) {
	store, state := campaign(t)
	state = apply(t, store, state, Command{ID: "build", Kind: "edict", Edict: &EdictPlan{Text: "建造水利", Instructions: []Instruction{order("water", "construction", "shaanxi", "fan-jingwen", 200000, 0, 4)}}}).State
	state = quarter(t, store, state, "q1")
	pid := state.Projects[0].ID
	prior := state.Treasury
	state = apply(t, store, state, Command{ID: "amend", Kind: "edict", Edict: &EdictPlan{Text: "暂停，追加五万两，改派毕自严", Instructions: []Instruction{{ID: "pause", Kind: "pause", ProjectID: pid}, {ID: "fund", Kind: "add_funds", ProjectID: pid, Budget: 50000}, {ID: "assign", Kind: "reassign", ProjectID: pid, MinisterID: "bi-ziyan"}}}}).State
	if state.Treasury != prior-50000 || state.Projects[0].Budget != 250000 || state.Projects[0].MinisterID != "bi-ziyan" {
		t.Fatal("amendment not applied")
	}
	state = quarter(t, store, state, "q2")
	if state.Projects[0].Progress != 100 {
		t.Fatal("paused project progressed")
	}
	state = apply(t, store, state, Command{ID: "resume", Kind: "edict", Edict: &EdictPlan{Text: "续办", Instructions: []Instruction{{ID: "resume", Kind: "resume", ProjectID: pid}}}}).State
	state = quarter(t, store, state, "q3")
	if state.Projects[0].Progress != 200 || state.Projects[0].Spent <= 50000 {
		t.Fatal("additional funding stalled prior work")
	}
	state = apply(t, store, state, Command{ID: "huge", Kind: "edict", Edict: &EdictPlan{Text: "巨额工程", Instructions: []Instruction{order("huge", "construction", "shanxi", "fan-jingwen", 1000000000, 0, 4)}}}).State
	privy := state.PrivyPurse
	state = quarter(t, store, state, "q4")
	last := state.Projects[len(state.Projects)-1]
	if last.Status != "blocked" || last.Reserved || state.PrivyPurse != privy || state.Treasury < 0 {
		t.Fatal("insufficient funding borrowed or produced effects")
	}
}

func TestBoundedAIChangesExecutionAndInvalidPlanIsAtomic(t *testing.T) {
	store, state := campaign(t)
	state = apply(t, store, state, Command{ID: "build", Kind: "edict", Edict: &EdictPlan{Text: "兴修水利", Instructions: []Instruction{order("water", "construction", "shaanxi", "bi-ziyan", 200000, 0, 4)}}}).State
	pid := state.Projects[0].ID
	normal, _, err := PreviewAdvance(state, QuarterPlan{})
	if err != nil {
		t.Fatal(err)
	}
	fast, report, err := PreviewAdvance(state, QuarterPlan{Adjustments: []ProjectAdjustment{{ProjectID: pid, EfficiencyPct: 125, Reason: "及时协调役夫"}}, Events: []RegionalEvent{{RegionID: "shaanxi", SeverityDelta: -3, Reason: "本季雨水改善"}}})
	if err != nil {
		t.Fatal(err)
	}
	if fast.Projects[0].Progress <= normal.Projects[0].Progress || fast.Regions[2].Disaster >= normal.Regions[2].Disaster || report.Spent <= 50000 {
		t.Fatal("AI plan cosmetic only")
	}
	_, err = store.Apply(context.Background(), state.ID, Command{ID: "invalid", ExpectedRevision: state.Revision, Kind: "advance", Quarter: &QuarterPlan{Adjustments: []ProjectAdjustment{{ProjectID: pid, EfficiencyPct: 999, Reason: "非法赠送"}}}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected bounded rejection: %v", err)
	}
	fresh, err := store.Read(context.Background(), state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(state, fresh) {
		t.Fatal("failed quarter partially committed")
	}
}

func TestDurableIdempotenceConflictAndCancellation(t *testing.T) {
	store, state := campaign(t)
	c := Command{ID: "next", Kind: "advance", ExpectedRevision: state.Revision, RequestHash: "original-user-request", Quarter: &QuarterPlan{}}
	first, err := store.Apply(context.Background(), state.ID, c)
	if err != nil {
		t.Fatal(err)
	}
	store = NewStore(filepath.Dir(store.root))
	second, err := store.Apply(context.Background(), state.ID, c)
	if err != nil || !second.Duplicate || second.State.Turn != 1 || second.Revision != first.Revision {
		t.Fatalf("duplicate changed state: %v %+v", err, second)
	}
	c.RequestHash = "different-user-request"
	if _, err = store.Apply(context.Background(), state.ID, c); !errors.Is(err, ErrCommandConflict) {
		t.Fatal("reused ID not rejected")
	}
	c.ID = "stale"
	if _, err = store.Apply(context.Background(), state.ID, c); !errors.Is(err, ErrConflict) {
		t.Fatal("stale revision not rejected")
	}
	path, _ := store.path(state.ID)
	before, _ := os.ReadFile(path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.ID = "canceled"
	c.ExpectedRevision = first.State.Revision
	if _, err = store.Apply(ctx, state.ID, c); !errors.Is(err, context.Canceled) {
		t.Fatalf("wrong canceled result: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("cancellation changed journal")
	}
}

func TestConcurrentDuplicateCommandsCommitExactlyOnce(t *testing.T) {
	store, state := campaign(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	receipts := make(chan Receipt, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if p := recover(); p != nil {
					errs <- fmt.Errorf("panic: %v", p)
				}
			}()
			r, err := NewStore(filepath.Dir(store.root)).Apply(context.Background(), state.ID, Command{ID: "same", ExpectedRevision: state.Revision, Kind: "advance", Quarter: &QuarterPlan{}})
			if err != nil {
				errs <- err
			} else {
				receipts <- r
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	fresh, err := store.Read(context.Background(), state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Turn != 1 || len(fresh.Reports) != 1 || len(receipts) != 2 {
		t.Fatal("concurrent command duplicated settlement")
	}
}

func TestMemoryScopeFreshFactsAndCheckpointBranch(t *testing.T) {
	store, state := campaign(t)
	chat := func(id, ministerID, message string) State {
		return apply(t, store, state, Command{ID: id, Kind: "chat", Chat: &ChatTurn{Mode: "private", MinisterIDs: []string{ministerID}, UserMessage: message, Replies: []Reply{{MinisterID: ministerID, Text: "臣承诺负责此事。"}}}}).State
	}
	state = chat("secret", "wang-chengen", "密议：暗号青灯，内帑不得外传")
	state = chat("finance", "bi-ziyan", "你刚说国库一百万，承诺逐季核账")
	state = apply(t, store, state, Command{ID: "save", Kind: "checkpoint", Label: "开局密谈后"}).State
	checkpointID := state.Checkpoints[0].ID
	savedTreasury := state.Treasury
	state = chat("discard", "bi-ziyan", "废弃分支：任命密使")
	state = quarter(t, store, state, "q1")
	c, err := store.Context(context.Background(), state.ID, "bi-ziyan", "核账承诺")
	if err != nil {
		t.Fatal(err)
	}
	if c.CurrentFacts["treasury"] != state.Treasury || c.CurrentFacts["privy_purse"] != nil || len(c.Memories) != 2 || c.Memories[0].ID != "finance" {
		t.Fatalf("context scope/freshness wrong: %+v", c)
	}
	state = apply(t, store, state, Command{ID: "restore", Kind: "restore", CheckpointID: checkpointID}).State
	if state.Turn != 0 || state.Treasury != savedTreasury || len(state.Chats) != 3 || len(state.Reports) != 1 {
		t.Fatal("restore destroyed raw audit or failed to rewind game")
	}
	c, err = store.Context(context.Background(), state.ID, "bi-ziyan", "密使")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Memories) != 1 || c.Memories[0].ID != "finance" {
		t.Fatal("discarded branch became current memory")
	}
	state = chat("new-branch", "bi-ziyan", "新分支承诺")
	c, err = store.Context(context.Background(), state.ID, "bi-ziyan", "承诺")
	if err != nil || len(c.Memories) != 2 {
		t.Fatal("post-restore memory omitted")
	}
}

func TestDependencyCyclesAndDenseGraphBounded(t *testing.T) {
	_, state := campaign(t)
	plan := EdictPlan{Text: "分期工程"}
	for i := 0; i < 32; i++ {
		in := order(fmt.Sprintf("p%d", i), "construction", "shanxi", "fan-jingwen", 20000, 0, 1)
		for j := 0; j < i; j++ {
			in.DependsOn = append(in.DependsOn, fmt.Sprintf("p%d", j))
		}
		plan.Instructions = append(plan.Instructions, in)
	}
	started := time.Now()
	out := ValidatePlan(state, plan)
	if time.Since(started) > time.Second {
		t.Fatal("dense graph validation exceeded one second")
	}
	for _, o := range out {
		if o.Status != "ready" {
			t.Fatalf("valid DAG rejected: %+v", o)
		}
	}
	plan.Instructions[0].DependsOn = []string{"p31"}
	out = ValidatePlan(state, plan)
	for _, o := range out {
		if o.Status != "unsupported" {
			t.Fatal("cyclic dependency allowed")
		}
	}
}

func TestReportInspectionUsesCanonicalNumbers(t *testing.T) {
	store, state := campaign(t)
	state = quarter(t, store, state, "q1")
	state = apply(t, store, state, Command{ID: "audit", Kind: "inspect", InspectionTarget: "treasury"}).State
	if len(state.Inspections) != 1 || state.Inspections[0].Facts["treasury"] != float64(state.Treasury) {
		t.Fatalf("inspection did not persist authoritative numbers: %+v", state.Inspections)
	}
}

func TestRestoreLaterCheckpointReactivatesItsOwnWitnessedHistory(t *testing.T) {
	store, state := campaign(t)
	state = apply(t, store, state, Command{ID: "save-a", Kind: "checkpoint", Label: "A"}).State
	a := state.Checkpoints[0].ID
	state = apply(t, store, state, Command{ID: "talk-b", Kind: "chat", Chat: &ChatTurn{Mode: "private", MinisterIDs: []string{"bi-ziyan"}, UserMessage: "乙分支承诺", Replies: []Reply{{MinisterID: "bi-ziyan", Text: "承诺核账"}}}}).State
	state = apply(t, store, state, Command{ID: "save-b", Kind: "checkpoint", Label: "B"}).State
	b := state.Checkpoints[1].ID
	state = apply(t, store, state, Command{ID: "back-a", Kind: "restore", CheckpointID: a}).State
	if !state.Chats[0].Discarded {
		t.Fatal("old branch not labeled")
	}
	state = apply(t, store, state, Command{ID: "back-b", Kind: "restore", CheckpointID: b}).State
	c, err := store.Context(context.Background(), state.ID, "bi-ziyan", "乙分支")
	if err != nil {
		t.Fatal(err)
	}
	if state.Chats[0].Discarded || len(c.Memories) != 1 {
		t.Fatal("restoring a saved later branch lost its witnessed history")
	}
}

func TestJournalColdReplayWithoutSidecarAndTamperDetection(t *testing.T) {
	store, state := campaign(t)
	state = quarter(t, store, state, "q1")
	path, _ := store.path(state.ID)
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "journal.jsonl" && !entry.IsDir() {
			if err := os.Remove(filepath.Join(filepath.Dir(path), entry.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	rebuilt, err := store.Read(context.Background(), state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(state, rebuilt) {
		t.Fatal("canonical journal did not reproduce state")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Change only the initial snapshot, leaving valid JSON. Its domain checksum
	// protects the first record, before the shared substrate's hash chain begins.
	modified := bytes.Replace(raw, []byte(`"treasury":1000000`), []byte(`"treasury":9000000`), 1)
	if bytes.Equal(raw, modified) {
		t.Fatal("seed marker missing")
	}
	if err = os.WriteFile(path, modified, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Read(context.Background(), state.ID); err == nil {
		t.Fatal("tampered first event accepted")
	}
}

func TestCreateRetriesAndConcurrentCreateAreOneCampaign(t *testing.T) {
	store := NewStore(t.TempDir())
	req := CreateRequest{Name: "同一开局", CommandID: "create-same"}
	var wg sync.WaitGroup
	results := make(chan State, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if p := recover(); p != nil {
					errs <- fmt.Errorf("panic: %v", p)
				}
			}()
			s, err := store.Create(context.Background(), req)
			if err != nil {
				errs <- err
			} else {
				results <- s
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	first, second := <-results, <-results
	if first.ID != second.ID || first.Revision != 1 || second.Revision != 1 {
		t.Fatal("concurrent creation duplicated campaign")
	}
	req.Name = "changed"
	if _, err := store.Create(context.Background(), req); !errors.Is(err, ErrCommandConflict) {
		t.Fatal("reused create command accepted different payload")
	}
	list, err := store.List(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("campaign list mismatch: %v", err)
	}
}

func TestInspectionFromDiscardedQuarterIsMarked(t *testing.T) {
	store, state := campaign(t)
	state = apply(t, store, state, Command{ID: "save", Kind: "checkpoint", Label: "原点"}).State
	checkpoint := state.Checkpoints[0].ID
	state = quarter(t, store, state, "q1")
	state = apply(t, store, state, Command{ID: "inspect", Kind: "inspect", InspectionTarget: "treasury"}).State
	state = apply(t, store, state, Command{ID: "restore", Kind: "restore", CheckpointID: checkpoint}).State
	if len(state.Inspections) != 1 || !state.Inspections[0].Discarded || state.Inspections[0].Revision == 0 {
		t.Fatal("discarded audit appeared current")
	}
}

func TestEventChecksumAllowsOmittedNewOptionalFields(t *testing.T) {
	state := initialState("test", "old optional schema")
	state.Revision = 1
	e := journalEvent{Version: 1, State: coreState(state), Receipt: storedReceipt{ID: "create", Kind: "create", Revision: 1}, Chat: &ChatTurn{ID: "old-chat", Mode: "private", MinisterIDs: []string{"bi-ziyan"}, UserMessage: "原话", Replies: []Reply{{MinisterID: "bi-ziyan", Text: "记住了"}}}}
	raw, err := marshalEvent(e)
	if err != nil {
		t.Fatal(err)
	}
	var encoded struct {
		SHA256 string `json:"sha256"`
	}
	if err = json.Unmarshal(raw, &encoded); err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte(`"sha256":"`+encoded.SHA256+`"`), []byte(`"sha256":""`), 1)
	raw = bytes.ReplaceAll(raw, []byte(`"discarded":false,`), nil)
	checksum := sha256.Sum256(raw)
	raw = bytes.Replace(raw, []byte(`"sha256":""`), []byte(`"sha256":"`+hex.EncodeToString(checksum[:])+`"`), 1)
	p := &projection{}
	if err = p.Reset(); err != nil {
		t.Fatal(err)
	}
	if err = p.Apply(conversationjournal.Record{Location: conversationjournal.Location{Cursor: 1}, Payload: raw}); err != nil {
		t.Fatalf("optional-field addition broke existing history: %v", err)
	}
}

func TestFractionalQuarterTranchesRetainCumulativeBenefits(t *testing.T) {
	one := initialState("one", "one")
	many := initialState("many", "many")
	if _, err := applyEdict(&one, "edict", EdictPlan{Text: "操练", Instructions: []Instruction{order("train", "training", "liaodong", "sun-chengzong", 10000, 0, 1)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := applyEdict(&many, "edict", EdictPlan{Text: "四季操练", Instructions: []Instruction{order("train", "training", "liaodong", "sun-chengzong", 10000, 0, 4)}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		var err error
		one, _, err = PreviewAdvance(one, QuarterPlan{})
		if err != nil {
			t.Fatal(err)
		}
		many, _, err = PreviewAdvance(many, QuarterPlan{})
		if err != nil {
			t.Fatal(err)
		}
	}
	if one.Regions[1].Training != 27 || many.Regions[1].Training != 27 {
		t.Fatalf("quarter tranches lost training: %d vs %d", one.Regions[1].Training, many.Regions[1].Training)
	}
	relief := initialState("relief", "relief")
	relief.Regions[2].Disaster = 10
	if _, err := applyEdict(&relief, "edict", EdictPlan{Text: "分四季救济", Instructions: []Instruction{order("relief", "relief", "shaanxi", "bi-ziyan", 40000, 7000, 4)}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		var err error
		relief, _, err = PreviewAdvance(relief, QuarterPlan{})
		if err != nil {
			t.Fatal(err)
		}
	}
	// 7000 grain crosses three 2000 thresholds even though each tranche is1750.
	// Four turns add eight natural severity;40000 silver earns two morale points.
	if relief.Regions[2].Disaster != 15 || relief.Regions[2].Heart != 38 {
		t.Fatalf("fractional relief benefits lost: %+v", relief.Regions[2])
	}
}

func TestOneCorruptCampaignDoesNotHideHealthyCampaigns(t *testing.T) {
	store := NewStore(t.TempDir())
	states := []State{}
	for i := 0; i < 4; i++ {
		state, err := store.Create(context.Background(), CreateRequest{Name: fmt.Sprintf("campaign%d", i), CommandID: fmt.Sprintf("create%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		states = append(states, state)
	}
	brokenPath, _ := store.path(states[0].ID)
	bad := []byte(`{"broken":true}`)
	if err := os.WriteFile(brokenPath, bad, 0600); err != nil {
		t.Fatal(err)
	}
	list, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	goodCount, badCount := 0, 0
	for _, item := range list {
		if item.Unavailable {
			badCount++
			if item.ID != states[0].ID || item.ErrorCode != "history.campaign_unreadable" {
				t.Fatal("unreadable marker missing")
			}
		} else {
			goodCount++
		}
	}
	if goodCount != 3 || badCount != 1 {
		t.Fatalf("catalog hid saves: %+v", list)
	}
	preserved, err := os.ReadFile(brokenPath)
	if err != nil || !bytes.Equal(preserved, bad) {
		t.Fatal("listing altered corrupt canonical data")
	}
	if _, err = store.Create(context.Background(), CreateRequest{Name: "new after corruption", CommandID: "new-good"}); err != nil {
		t.Fatal("corrupt sibling blocked new campaign")
	}
}

func TestQuarterPersistsOnlyApplicableAICauses(t *testing.T) {
	store, state := campaign(t)
	state = apply(t, store, state, Command{ID: "edict", Kind: "edict", Edict: &EdictPlan{Text: "分项办理", Instructions: []Instruction{order("active", "construction", "shaanxi", "fan-jingwen", 200000, 0, 4), order("paused", "construction", "shanxi", "bi-ziyan", 200000, 0, 4), order("blocked", "construction", "beizhili", "bi-ziyan", 1000000000, 0, 4)}}}).State
	state = apply(t, store, state, Command{ID: "pause", Kind: "edict", Edict: &EdictPlan{Text: "暂停第二项", Instructions: []Instruction{{ID: "pause", Kind: "pause", ProjectID: state.Projects[1].ID}}}}).State
	plan := QuarterPlan{Adjustments: []ProjectAdjustment{{ProjectID: state.Projects[0].ID, EfficiencyPct: 100, Delay: true, Reason: "暴雨冲断木料运输道路"}, {ProjectID: state.Projects[1].ID, EfficiencyPct: 125, Reason: "暂停项不应声称加速"}, {ProjectID: state.Projects[2].ID, EfficiencyPct: 75, Delay: true, Reason: "缺钱项不应声称交通受阻"}}}
	state = apply(t, store, state, Command{ID: "quarter", Kind: "advance", Quarter: &plan}).State
	fresh, err := NewStore(filepath.Dir(store.root)).Read(context.Background(), state.ID)
	if err != nil {
		t.Fatal(err)
	}
	report := fresh.Reports[0]
	if len(report.AppliedAdjustments) != 1 || report.AppliedAdjustments[0].Reason != "暴雨冲断木料运输道路" || report.Outcomes[0].Status != "delayed" || report.Outcomes[2].Reason != "insufficient_funding" {
		t.Fatalf("AI cause provenance lost or overclaimed: %+v", report)
	}
	if fresh.Projects[0].Reserved {
		t.Fatal("delayed project reserved new money")
	}
}

func TestEdictRetainsLiveChatSourceAndRejectsUnknownOrDiscardedSources(t *testing.T) {
	store, state := campaign(t)
	state = apply(t, store, state, Command{ID: "save-before-chat", Kind: "checkpoint", Label: "讨论前"}).State
	checkpoint := state.Checkpoints[0].ID
	state = apply(t, store, state, Command{ID: "source-chat", Kind: "chat", Chat: &ChatTurn{Mode: "private", MinisterIDs: []string{"bi-ziyan"}, UserMessage: "请拟赈济方案", Replies: []Reply{{MinisterID: "bi-ziyan", Text: "建议拨银五万、发粮三万石赈济陕西，一季办理。"}}}}).State
	plan := EdictPlan{SourceChatID: "source-chat", Text: "按爱卿所议办理", Instructions: []Instruction{order("relief", "relief", "shaanxi", "bi-ziyan", 50000, 30000, 1)}}
	state = apply(t, store, state, Command{ID: "edict-from-chat", Kind: "edict", Edict: &plan}).State
	cold, err := NewStore(filepath.Dir(store.root)).Read(context.Background(), state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cold.Edicts[0].Plan.SourceChatID != "source-chat" {
		t.Fatal("decree source provenance lost")
	}
	plan.SourceChatID = "another-campaign-chat"
	if _, err = store.Apply(context.Background(), state.ID, Command{ID: "unknown-source", ExpectedRevision: state.Revision, Kind: "edict", Edict: &plan}); !errors.Is(err, ErrInvalid) {
		t.Fatal("foreign or unknown source accepted")
	}
	state = apply(t, store, state, Command{ID: "restore-before-chat", Kind: "restore", CheckpointID: checkpoint}).State
	plan.SourceChatID = "source-chat"
	if _, err = store.Apply(context.Background(), state.ID, Command{ID: "discarded-source", ExpectedRevision: state.Revision, Kind: "edict", Edict: &plan}); !errors.Is(err, ErrInvalid) {
		t.Fatal("discarded source authorized new decree")
	}
	unchanged, err := store.Read(context.Background(), state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Revision != state.Revision || len(unchanged.Edicts) != 0 {
		t.Fatal("invalid source partially committed")
	}
}
