package history

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func worldCampaign(t *testing.T) (*Store, State) {
	t.Helper()
	store := NewStore(t.TempDir())
	s, err := store.Create(context.Background(), CreateRequest{Name: "World integration", CommandID: "create-world", ScenarioID: "ming-1627", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	return store, s
}
func worldIssue(t *testing.T, store *Store, s State, cmd string, intents ...OrderIntent) State {
	t.Helper()
	r, err := store.Apply(context.Background(), s.ID, Command{ID: cmd, Kind: "edict", ExpectedRevision: s.Revision, Edict: &EdictPlan{Text: "An authorized decree", Orders: intents}})
	if err != nil {
		t.Fatal(err)
	}
	return r.State
}
func quietProposal(s State) *WorldProposal {
	p := &WorldProposal{}
	for _, o := range s.World.Orders {
		if orderActive(o.Status) {
			status := "active"
			if !orderReady(s.World, o, s.Turn) || o.Condition != "" {
				status = "waiting"
			}
			p.Results = append(p.Results, OrderResult{OrderID: o.ID, Status: status, Progress: o.Progress, Reason: "Continuing as ordered", ConditionMet: o.Condition == ""})
		}
	}
	return p
}

func TestWorldPacksAreCompleteFrozenAndDistinct(t *testing.T) {
	store, s := worldCampaign(t)
	packs, err := store.Scenarios()
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 2 {
		t.Fatalf("packs: %d", len(packs))
	}
	if len(s.World.Entities) < 190 || len(s.Ministers) != 102 || s.ScenarioDigest == "" {
		t.Fatal("Ming full source data missing")
	}
	song, err := store.Create(context.Background(), CreateRequest{Name: "Song", CommandID: "song", ScenarioID: "song-1127"})
	if err != nil {
		t.Fatal(err)
	}
	if song.Year != 1127 || song.World.Accounts[0].Resource != "cash" || song.Scenario.Calendar.EraOffset != 1126 {
		t.Fatal("scenario-specific era or currency lost")
	}
	custom := *s.Scenario
	custom.ID = "custom-frozen"
	b, _ := json.Marshal(custom)
	if _, err = store.ImportScenario(b); err != nil {
		t.Fatal(err)
	}
	x, err := store.Create(context.Background(), CreateRequest{Name: "Custom", CommandID: "custom", ScenarioID: custom.ID})
	if err != nil {
		t.Fatal(err)
	}
	custom.Initial.Accounts[0].Balance = 2
	b, _ = json.Marshal(custom)
	if _, err = store.ImportScenario(b); !errors.Is(err, ErrConflict) {
		t.Fatal("pack overwrite must fail")
	}
	if err = os.Remove(filepath.Join(store.root, "scenarios", custom.ID+".json")); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Read(context.Background(), x.ID)
	if err != nil || reopened.World.Accounts[0].Balance != 1000000 {
		t.Fatalf("frozen campaign changed: %v", err)
	}
	custom.ID = "../escape"
	b, _ = json.Marshal(custom)
	if _, err = store.ImportScenario(b); err == nil {
		t.Fatal("path traversal accepted")
	}
}

func TestWorldOrdersQuarterAccountingRetryAndRestore(t *testing.T) {
	ctx := context.Background()
	store, s := worldCampaign(t)
	s = worldIssue(t, store, s, "issue", OrderIntent{Action: "create", Kind: "policy", Text: "Fund relief each season up to the total ceiling", MinisterID: "bi-ziyan", Targets: []string{"shaanxi"}, Limits: map[string]int64{"silver": 1000}})
	o := s.World.Orders[0]
	p := quietProposal(s)
	p.Transfers = []Transfer{{From: "treasury", Resource: "silver", Amount: 300, OrderID: o.ID, Reason: "Relief administration"}, {To: "treasury", Resource: "silver", Amount: 500, Reason: "Collected taxes"}}
	p.Changes = []EntityChange{{ID: "shaanxi", MetricDeltas: map[string]int64{"heart": 2, "refugees": -50}, Reason: "Relief reaches displaced households"}}
	p.Events = []WorldEvent{{Title: "Relief delivered", Text: "The project report reached its responsible minister.", EntityIDs: []string{"shaanxi"}, OrderIDs: []string{o.ID}, Audience: []string{"player", "bi-ziyan"}, Status: "resolved"}}
	cmd := Command{ID: "quarter", Kind: "advance", ExpectedRevision: s.Revision, Quarter: &QuarterPlan{World: p, Narrative: "Verified quarterly report"}}
	r, err := store.Apply(ctx, s.ID, cmd)
	if err != nil {
		t.Fatal(err)
	}
	s = r.State
	if s.World.Accounts[0].Balance != 1000200 || s.World.Orders[0].Spent["silver"] != 300 || s.World.Orders[0].Status != "active" || s.Turn != 1 {
		t.Fatal("settlement accounting or policy continuity failed")
	}
	r, err = store.Apply(ctx, s.ID, cmd)
	if err != nil || !r.Duplicate || r.State.Turn != 1 {
		t.Fatal("retry settled a second time")
	}
	checkpoint, err := store.Apply(ctx, s.ID, Command{ID: "checkpoint", Kind: "checkpoint", ExpectedRevision: s.Revision, Label: "After relief"})
	if err != nil {
		t.Fatal(err)
	}
	s = checkpoint.State
	s = worldIssue(t, store, s, "pause", OrderIntent{Action: "pause", OrderID: o.ID})
	if len(quietProposal(s).Results) != 0 {
		t.Fatal("paused order scheduled")
	}
	s = worldIssue(t, store, s, "resume", OrderIntent{Action: "resume", OrderID: o.ID})
	p = quietProposal(s)
	p.Transfers = []Transfer{{From: "treasury", Resource: "silver", Amount: 701, OrderID: o.ID, Reason: "Exceeds remaining authorization"}}
	before := digest(s)
	_, err = store.Apply(ctx, s.ID, Command{ID: "overspend", Kind: "advance", ExpectedRevision: s.Revision, Quarter: &QuarterPlan{World: p}})
	if err == nil {
		t.Fatal("overspend accepted")
	}
	unchanged, _ := store.Read(ctx, s.ID)
	if digest(unchanged) != before {
		t.Fatal("failed quarter mutated state")
	}
	_, err = store.Apply(ctx, s.ID, Command{ID: "stale", Kind: "advance", ExpectedRevision: 1, Quarter: &QuarterPlan{World: quietProposal(s)}})
	if !errors.Is(err, ErrConflict) {
		t.Fatal("stale revision accepted")
	}
	r, err = store.Apply(ctx, s.ID, Command{ID: "restore", Kind: "restore", ExpectedRevision: s.Revision, CheckpointID: checkpoint.State.Checkpoints[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if r.State.World.Orders[0].Version != 1 || r.State.World.Accounts[0].Balance != 1000200 || r.State.World.Orders[0].Spent["silver"] != 300 {
		t.Fatal("restore failed to recover full world")
	}
	context, err := store.Context(ctx, s.ID, "bi-ziyan", "relief")
	if err != nil {
		t.Fatal(err)
	}
	view := context.CurrentFacts["world"].(WorldContext)
	if len(view.Events) < 1 || view.Events[len(view.Events)-1].Title != "Relief delivered" {
		t.Fatal("minister did not receive execution result")
	}
}

func TestWorldConditionalDependenciesAndPrivateProjection(t *testing.T) {
	store, s := worldCampaign(t)
	s = worldIssue(t, store, s, "first", OrderIntent{Action: "create", Kind: "project", Text: "Prepare supplies", Targets: []string{"shaanxi"}})
	first := s.World.Orders[0].ID
	s = worldIssue(t, store, s, "second", OrderIntent{Action: "create", Kind: "conditional", Text: "Deploy only after preparation and inspection", Condition: "Inspection confirms readiness", DependsOn: []string{first}, Targets: []string{"shaanxi"}})
	p := quietProposal(s)
	p.Results[0].Status = "completed"
	p.Results[0].Progress = 100
	p.Results[1].Status = "active"
	p.Results[1].ConditionMet = true
	if _, _, err := PreviewWorld(s, QuarterPlan{World: p}); err == nil {
		t.Fatal("same-quarter dependency bypass")
	}
	p.Results[1].Status = "waiting"
	next, _, err := PreviewWorld(s, QuarterPlan{World: p})
	if err != nil {
		t.Fatal(err)
	}
	if next.World.Orders[1].Status != "waiting" {
		t.Fatal("conditional order did not wait")
	}
	next.World.Events = append(next.World.Events, WorldEvent{ID: "private", Title: "SECRET-TEST", Text: "Private report", Audience: []string{"wang-chengen"}})
	next.World.Entities = append(next.World.Entities, entitySecret())
	next.Scenario.Initial.Entities = append(next.Scenario.Initial.Entities, entitySecret())
	visible := PlayerState(next)
	data, _ := json.Marshal(visible)
	if strings.Contains(string(data), "SECRET-TEST") {
		t.Fatal("hidden truth leaked to player")
	}
	ministerView := VisibleWorld(*next.World, "wang-chengen")
	if len(ministerView.Events) != len(next.World.Events) {
		t.Fatal("delivered private report omitted")
	}
}

func entitySecret() Entity {
	return Entity{ID: "hidden-fact", Kind: "intelligence", Name: Text{ZH: "SECRET-TEST", EN: "SECRET-TEST"}, Status: "active", Audience: []string{}}
}

func TestWorldArchiveRecoveryAndEnglishOpening(t *testing.T) {
	ctx := context.Background()
	store, s := worldCampaign(t)
	if s.World.Events[0].Title != "Drought in Shaanxi" {
		t.Fatal("English opening was not localized")
	}
	r, err := store.Apply(ctx, s.ID, Command{ID: "archive", ExpectedRevision: s.Revision, Kind: "archive"})
	if err != nil || !r.State.Archived {
		t.Fatal("archive failed")
	}
	list, err := store.List(ctx)
	if err != nil || len(list) != 1 || !list[0].Archived {
		t.Fatal("recoverable archive missing")
	}
	_, err = store.Apply(ctx, s.ID, Command{ID: "advance-archived", ExpectedRevision: r.State.Revision, Kind: "advance", Quarter: &QuarterPlan{World: quietProposal(r.State)}})
	if !errors.Is(err, ErrNotFound) {
		t.Fatal("archived campaign was mutated")
	}
	recovered, err := store.Apply(ctx, s.ID, Command{ID: "recover", ExpectedRevision: r.State.Revision, Kind: "unarchive"})
	if err != nil || recovered.State.Archived || recovered.State.World.Accounts[0].Balance != s.World.Accounts[0].Balance {
		t.Fatal("recovery lost campaign")
	}
}

func TestWorldEventUpdatesKeepIdentityAndOldDependenciesRemainInContext(t *testing.T) {
	_, s := worldCampaign(t)
	id := s.World.Events[0].ID
	p := quietProposal(s)
	p.Events = []WorldEvent{{ID: id, Title: "Drought eases", Text: "Rain has reduced the immediate pressure.", Audience: []string{"public"}, Status: "resolved"}}
	next, _, err := PreviewWorld(s, QuarterPlan{World: p})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.World.Events) != 1 || next.World.Events[0].ID != id || next.World.Events[0].Title != "Drought eases" {
		t.Fatal("event update duplicated the crisis")
	}
	next.World.Orders = []Order{{ID: "old-dependency", Version: 1, Status: "completed", Progress: 100}}
	for i := 0; i < 30; i++ {
		next.World.Orders = append(next.World.Orders, Order{ID: shortID("closed-", string(rune('a'+i))), Version: 1, Status: "completed", Progress: 100})
	}
	next.World.Orders = append(next.World.Orders, Order{ID: "active-dependent", Version: 1, Status: "active", DependsOn: []string{"old-dependency"}})
	b, _ := json.Marshal(SimulationFacts(next))
	var facts struct {
		World World `json:"world"`
	}
	if err = json.Unmarshal(b, &facts); err != nil {
		t.Fatal(err)
	}
	if orderByID(&facts.World, "old-dependency") == nil {
		t.Fatal("old completed prerequisite was omitted from simulation context")
	}
}

func TestWorldNewGranaryTransfersDoNotMintOrDoubleCharge(t *testing.T) {
	store, s := worldCampaign(t)
	s = worldIssue(t, store, s, "granary", OrderIntent{Action: "create", Kind: "project", Text: "Open a local grain reserve, with consumption capped at 100 shi", Targets: []string{"shaanxi"}, Limits: map[string]int64{"grain": 100}})
	o := s.World.Orders[0]
	p := quietProposal(s)
	p.NewAccounts = []Account{{ID: "local-granary", Name: Text{ZH: "陕西仓", EN: "Shaanxi granary"}, Resource: "grain", Owner: "shaanxi", Audience: []string{"player"}}}
	p.Transfers = []Transfer{{From: "granary", To: "local-granary", Resource: "grain", Amount: 100, OrderID: o.ID, Reason: "Deliver grain to the reserve"}, {From: "local-granary", Resource: "grain", Amount: 50, OrderID: o.ID, Reason: "Distribute food to households"}}
	next, _, err := PreviewWorld(s, QuarterPlan{World: p})
	if err != nil {
		t.Fatal(err)
	}
	if accountByID(next.World, "granary").Balance != 629900 || accountByID(next.World, "local-granary").Balance != 50 || next.World.Orders[0].Spent["grain"] != 50 {
		t.Fatal("transfer was charged as consumption or balances were not conserved")
	}
	p.NewAccounts[0].Balance = 100
	if _, _, err := PreviewWorld(s, QuarterPlan{World: p}); err == nil {
		t.Fatal("new account minted inventory")
	}
}

func TestWorldContextKeepsUnresolvedFactsWithoutGrowingWithClosedArchive(t *testing.T) {
	w := World{Events: []WorldEvent{{ID: "old-open", Status: "open", Text: "Still unresolved"}}}
	for i := 0; i < 1000; i++ {
		w.Events = append(w.Events, WorldEvent{ID: shortID("closed-", string(rune(i+1))), Status: "resolved", Text: "A settled old event"})
	}
	view := ContextWorld(w)
	if len(view.Events) != 25 || view.Events[0].ID != "old-open" || len(w.Events) != 1001 {
		t.Fatal("bounded context lost an unresolved event or deleted raw history")
	}
}

func TestWorldCouncilSpeakerRetainsOwnOfficialFacts(t *testing.T) {
	_, s := worldCampaign(t)
	s.World.Orders = []Order{{ID: "relief", MinisterID: "bi-ziyan", Status: "active"}}
	s.World.Events = append(s.World.Events, WorldEvent{ID: "private", Title: "A private confidence", Audience: []string{"bi-ziyan"}})
	view := CouncilWorld(*s.World, []string{"bi-ziyan", "sun-chengzong"}, "bi-ziyan")
	if accountByID(&view, "treasury") == nil || accountByID(&view, "privy") != nil || len(view.Orders) != 1 {
		t.Fatal("council removed the speaker's official accounts or assigned work")
	}
	for _, event := range view.Events {
		if event.ID == "private" {
			t.Fatal("private narrative facts entered the shared context")
		}
	}
}

func TestWorldChineseInputUsesTheSameCharacterLimitsAsTheAPI(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	s, err := store.Create(ctx, CreateRequest{Name: strings.Repeat("政", 100), CommandID: "unicode-name", ScenarioID: "ming-1627"})
	if err != nil {
		t.Fatal(err)
	}
	message := strings.Repeat("请核实", 2600)
	r, err := store.Apply(ctx, s.ID, Command{ID: "unicode-chat", ExpectedRevision: s.Revision, Kind: "chat", Chat: &ChatTurn{Mode: "private", MinisterIDs: []string{"bi-ziyan"}, UserMessage: message, Replies: []Reply{{MinisterID: "bi-ziyan", Text: "臣已读完。"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if r.State.Chats[0].UserMessage != message {
		t.Fatal("Chinese input was truncated")
	}
}
