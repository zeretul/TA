package history

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"ta/internal/agents/conversationjournal"
	"ta/internal/localfs"
)

var safeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,100}$`)

type Store struct{ root string }

func NewStore(projectStoreDir string) *Store {
	return &Store{root: filepath.Join(projectStoreDir, "history")}
}

type storedReceipt struct {
	ID       string    `json:"id"`
	Hash     string    `json:"hash"`
	Kind     string    `json:"kind"`
	Revision uint64    `json:"revision"`
	Outcomes []Outcome `json:"outcomes"`
}
type checkpointRecord struct {
	Info  Checkpoint `json:"info"`
	State State      `json:"state"`
}
type journalEvent struct {
	SHA256     string            `json:"sha256"`
	Version    int               `json:"version"`
	State      State             `json:"state"`
	Receipt    storedReceipt     `json:"receipt"`
	Chat       *ChatTurn         `json:"chat,omitempty"`
	Report     *QuarterReport    `json:"report,omitempty"`
	Inspection *Inspection       `json:"inspection,omitempty"`
	Checkpoint *checkpointRecord `json:"checkpoint,omitempty"`
}
type projection struct {
	State     State                       `json:"state"`
	Receipts  map[string]storedReceipt    `json:"receipts"`
	Snapshots map[string]checkpointRecord `json:"snapshots"`
}

func (p *projection) Reset() error {
	p.State = State{}
	p.Receipts = map[string]storedReceipt{}
	p.Snapshots = map[string]checkpointRecord{}
	return nil
}
func (p *projection) Restore(b json.RawMessage) error      { return json.Unmarshal(b, p) }
func (p *projection) Checkpoint() (json.RawMessage, error) { return json.Marshal(p) }
func (p *projection) Apply(record conversationjournal.Record) error {
	var e journalEvent
	if err := json.Unmarshal(record.Payload, &e); err != nil {
		return err
	}
	// Validate original canonical bytes, not a remarshal through today's types:
	// additive optional fields must not invalidate an older recorded event.
	checksum := e.SHA256
	prefix := []byte(`{"sha256":"` + checksum + `"`)
	raw := bytes.TrimSpace(record.Payload)
	if len(checksum) != 64 || !bytes.HasPrefix(raw, prefix) {
		return errors.New("history event checksum missing")
	}
	unsigned := append([]byte(`{"sha256":""`), raw[len(prefix):]...)
	sum := sha256.Sum256(unsigned)
	if checksum != hex.EncodeToString(sum[:]) {
		return errors.New("history event checksum mismatch")
	}
	if e.Version != 1 {
		return errors.New("history unsupported journal event version")
	}
	if err := validateState(e.State); err != nil {
		return err
	}
	if e.State.Revision != uint64(record.Location.Cursor) {
		return errors.New("history revision does not match journal")
	}
	previous := p.State
	p.State = e.State
	p.State.Chats = previous.Chats
	p.State.Reports = previous.Reports
	p.State.Inspections = previous.Inspections
	p.State.Checkpoints = previous.Checkpoints
	if e.Chat != nil {
		p.State.Chats = append(p.State.Chats, *e.Chat)
	}
	if e.Report != nil {
		p.State.Reports = append(p.State.Reports, *e.Report)
	}
	if e.Inspection != nil {
		p.State.Inspections = append(p.State.Inspections, *e.Inspection)
	}
	if e.Checkpoint != nil {
		p.Snapshots[e.Checkpoint.Info.ID] = *e.Checkpoint
		p.State.Checkpoints = append(p.State.Checkpoints, e.Checkpoint.Info)
	}
	p.Receipts[e.Receipt.ID] = e.Receipt
	normalizeSlices(&p.State)
	return nil
}
func normalizeSlices(s *State) {
	for i := range s.Inspections {
		s.Inspections[i].Discarded = discarded(*s, s.Inspections[i].Revision)
	}
	for i := range s.Chats {
		s.Chats[i].Discarded = discarded(*s, s.Chats[i].Revision)
	}
	for i := range s.Reports {
		s.Reports[i].Discarded = discarded(*s, s.Reports[i].Revision)
	}
	if s.Chats == nil {
		s.Chats = []ChatTurn{}
	}
	if s.Reports == nil {
		s.Reports = []QuarterReport{}
	}
	if s.Projects == nil {
		s.Projects = []Project{}
	}
	if s.Edicts == nil {
		s.Edicts = []Edict{}
	}
	if s.Checkpoints == nil {
		s.Checkpoints = []Checkpoint{}
	}
	if s.Inspections == nil {
		s.Inspections = []Inspection{}
	}
	if s.DiscardedRanges == nil {
		s.DiscardedRanges = []RevisionRange{}
	}
}
func coreState(s State) State {
	s.Chats = nil
	s.Reports = nil
	s.Inspections = nil
	s.Checkpoints = nil
	return s
}
func (s *Store) path(id string) (string, error) {
	if !safeID.MatchString(id) {
		return "", invalid("history invalid campaign id")
	}
	return filepath.Join(s.root, id, "journal.jsonl"), nil
}
func (s *Store) open(ctx context.Context, id string) (*conversationjournal.Journal, *projection, error) {
	path, err := s.path(id)
	if err != nil {
		return nil, nil, err
	}
	if _, err = os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil, ErrNotFound
	} else if err != nil {
		return nil, nil, err
	}
	p := &projection{}
	j, err := conversationjournal.Open(ctx, path, conversationjournal.Identity{ID: id, Generation: "history-v1"}, p, conversationjournal.Options{})
	return j, p, err
}
func (s *Store) Read(ctx context.Context, id string) (State, error) {
	j, p, err := s.open(ctx, id)
	if err != nil {
		return State{}, err
	}
	defer j.Close()
	if p.State.ID == "" {
		return State{}, ErrNotFound
	}
	return cloneState(p.State), nil
}
func (s *Store) List(ctx context.Context) ([]Summary, error) {
	out := []Summary{}
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() || !safeID.MatchString(entry.Name()) {
			continue
		}
		state, err := s.Read(ctx, entry.Name())
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			out = append(out, Summary{ID: entry.Name(), Name: entry.Name(), Unavailable: true, ErrorCode: "history.campaign_unreadable"})
			continue
		}
		out = append(out, Summary{Archived: state.Archived, ID: state.ID, Name: state.Name, ScenarioID: state.ScenarioID, Revision: state.Revision, Year: state.Year, Quarter: state.Quarter, Turn: state.Turn})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func commandHash(c Command) string {
	if c.RequestHash != "" {
		return c.RequestHash
	}
	c.ExpectedRevision = 0
	return digest(c)
}
func receipt(p *projection, r storedReceipt, duplicate bool) Receipt {
	return Receipt{CommandID: r.ID, Revision: r.Revision, Kind: r.Kind, Duplicate: duplicate, State: cloneState(p.State), Outcomes: append([]Outcome{}, r.Outcomes...)}
}
func lookup(p *projection, id, hash string) (Receipt, bool, error) {
	r, ok := p.Receipts[id]
	if !ok {
		return Receipt{}, false, nil
	}
	if hash != "" && r.Hash != hash {
		return Receipt{}, false, ErrCommandConflict
	}
	return receipt(p, r, true), true, nil
}

// LookupCommand prevents expensive model retries; the hash describes original
// user input, excluding generated prose. The returned State is the current view.
func (s *Store) LookupCommand(ctx context.Context, id, commandID, requestHash string) (Receipt, bool, error) {
	j, p, err := s.open(ctx, id)
	if err != nil {
		return Receipt{}, false, err
	}
	defer j.Close()
	return lookup(p, commandID, requestHash)
}

// Create writes one complete, checksummed initial record before opening the
// shared substrate, whose first-record contract does not permit empty files.
func (s *Store) Create(ctx context.Context, req CreateRequest) (State, error) {
	if !safeID.MatchString(req.CommandID) || strings.TrimSpace(req.Name) == "" || utf8.RuneCountInString(req.Name) > 100 {
		return State{}, invalid("history create requires command id and name")
	}
	if req.ID == "" {
		req.ID = shortID("campaign-", req.CommandID)
	}
	path, err := s.path(req.ID)
	if err != nil {
		return State{}, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return State{}, err
	}
	release, err := localfs.AcquireLease(ctx, path+".create.lock")
	if err != nil {
		return State{}, err
	}
	defer release()
	hash := digest(req)
	if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
		state := initialState(req.ID, req.Name)
		if req.ScenarioID != "" && req.ScenarioID != "ming-1627-slice" {
			pack, err := s.Scenario(req.ScenarioID)
			if err != nil {
				return State{}, err
			}
			state = newScenarioState(req.ID, req.Name, req.Locale, pack)
		}
		state.Revision = 1
		e := journalEvent{Version: 1, State: coreState(state), Receipt: storedReceipt{ID: req.CommandID, Hash: hash, Kind: "create", Revision: 1, Outcomes: []Outcome{}}}
		data, err := marshalEvent(e)
		if err != nil {
			return State{}, err
		}
		file, err := os.CreateTemp(filepath.Dir(path), ".create-*.tmp")
		if err != nil {
			return State{}, err
		}
		tmp := file.Name()
		defer os.Remove(tmp)
		if _, err = file.Write(append(data, '\n')); err != nil {
			file.Close()
			return State{}, err
		}
		if err = file.Sync(); err != nil {
			file.Close()
			return State{}, err
		}
		if err = file.Close(); err != nil {
			return State{}, err
		}
		if err = ctx.Err(); err != nil {
			return State{}, err
		}
		if err = os.Rename(tmp, path); err != nil {
			return State{}, err
		}
	} else if statErr != nil {
		return State{}, statErr
	}
	j, p, err := s.open(ctx, req.ID)
	if err != nil {
		return State{}, err
	}
	defer j.Close()
	if existing, ok, err := lookup(p, req.CommandID, hash); ok || err != nil {
		return existing.State, err
	}
	return State{}, ErrConflict
}
func marshalEvent(e journalEvent) ([]byte, error) {
	e.SHA256 = ""
	e.SHA256 = digest(e)
	return json.Marshal(e)
}
func (s *Store) append(ctx context.Context, j *conversationjournal.Journal, e journalEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b, err := marshalEvent(e)
	if err != nil {
		return err
	}
	h := j.Head()
	_, err = j.Append(ctx, conversationjournal.Guard{Cursor: h.Cursor, RecordSHA256: h.RecordSHA256}, b)
	if errors.Is(err, conversationjournal.ErrConflict) {
		return ErrConflict
	}
	return err
}

func (s *Store) Apply(ctx context.Context, id string, c Command) (Receipt, error) {
	if !safeID.MatchString(c.ID) {
		return Receipt{}, invalid("history command id required")
	}
	j, p, err := s.open(ctx, id)
	if err != nil {
		return Receipt{}, err
	}
	defer j.Close()
	hash := commandHash(c)
	if old, ok, err := lookup(p, c.ID, hash); ok || err != nil {
		return old, err
	}
	if c.ExpectedRevision != p.State.Revision {
		return Receipt{}, ErrConflict
	}
	if p.State.Archived && c.Kind != "unarchive" {
		return Receipt{}, ErrNotFound
	}
	state := cloneState(p.State)
	nextRevision := state.Revision + 1
	out := []Outcome{}
	e := journalEvent{Version: 1}
	switch c.Kind {
	case "advice":
		if err := applyAdvice(&state, c.ID, c.Advice); err != nil {
			return Receipt{}, err
		}
	case "archive":
		state.Archived = true
	case "unarchive":
		state.Archived = false
	case "chat":
		if c.Chat == nil {
			return Receipt{}, invalid("history chat missing")
		}
		chat := *c.Chat
		if err = validateChat(state, chat); err != nil {
			return Receipt{}, err
		}
		chat.ID = c.ID
		chat.Turn = state.Turn
		chat.Revision = nextRevision
		e.Chat = &chat
	case "edict":
		if c.Edict == nil {
			return Receipt{}, invalid("history edict missing")
		}
		out, err = applyEdict(&state, c.ID, *c.Edict)
		if err != nil {
			return Receipt{}, err
		}
	case "advance":
		if c.Quarter == nil {
			return Receipt{}, invalid("history quarter plan missing")
		}
		var report QuarterReport
		state, report, err = PreviewAdvance(state, *c.Quarter)
		if err != nil {
			return Receipt{}, err
		}
		report.ID = c.ID
		report.Revision = nextRevision
		e.Report = &report
		out = report.Outcomes
	case "checkpoint":
		if strings.TrimSpace(c.Label) == "" || utf8.RuneCountInString(c.Label) > 100 {
			return Receipt{}, invalid("history checkpoint label required")
		}
		cid := c.CheckpointID
		if cid == "" {
			cid = shortID("checkpoint-", c.ID)
		}
		if !safeID.MatchString(cid) {
			return Receipt{}, invalid("history invalid checkpoint id")
		}
		if _, ok := p.Snapshots[cid]; ok {
			return Receipt{}, invalid("history checkpoint id exists")
		}
		e.Checkpoint = &checkpointRecord{Info: Checkpoint{ID: cid, Label: c.Label, Turn: state.Turn, Revision: nextRevision}, State: coreState(state)}
	case "restore":
		saved, ok := p.Snapshots[c.CheckpointID]
		if !ok {
			return Receipt{}, invalid("history checkpoint not found")
		}
		ranges := append([]RevisionRange{}, saved.State.DiscardedRanges...)
		ranges = append(ranges, RevisionRange{Start: saved.Info.Revision + 1, End: state.Revision})
		state = cloneState(saved.State)
		state.DiscardedRanges = ranges
		state.LastRestoreRevision = nextRevision
		out = append(out, Outcome{Status: "restored", Reason: "checkpoint_restored"})
	case "inspect":
		facts, err := inspectFacts(state, c.InspectionTarget)
		if err != nil {
			return Receipt{}, err
		}
		e.Inspection = &Inspection{ID: c.ID, Revision: nextRevision, Target: c.InspectionTarget, Turn: state.Turn, Facts: facts}
		out = append(out, Outcome{Status: "inspected", Reason: "canonical_records_checked"})
	default:
		return Receipt{}, invalid("history unsupported command kind")
	}
	state.Revision = nextRevision
	if err := validateState(state); err != nil {
		return Receipt{}, err
	}
	e.State = coreState(state)
	e.Receipt = storedReceipt{ID: c.ID, Hash: hash, Kind: c.Kind, Revision: nextRevision, Outcomes: out}
	if err = s.append(ctx, j, e); err != nil {
		if errors.Is(err, ErrConflict) {
			if old, ok, lookupErr := s.LookupCommand(ctx, id, c.ID, hash); ok || lookupErr != nil {
				return old, lookupErr
			}
		}
		return Receipt{}, err
	}
	return receipt(p, e.Receipt, false), nil
}

func validateChat(state State, c ChatTurn) error {
	if (c.Mode != "private" && c.Mode != "council") || len(c.MinisterIDs) == 0 || len(c.MinisterIDs) > len(state.Ministers) || strings.TrimSpace(c.UserMessage) == "" || utf8.RuneCountInString(c.UserMessage) > 12000 {
		return invalid("history invalid chat audience or message")
	}
	if c.Mode == "private" && len(c.MinisterIDs) != 1 {
		return invalid("history private chat requires one minister")
	}
	audience := map[string]bool{}
	for _, id := range c.MinisterIDs {
		if minister(state, id) == nil || audience[id] {
			return invalid("history invalid chat minister")
		}
		audience[id] = true
	}
	if len(c.Replies) != len(audience) {
		return invalid("history chat requires one reply per invited minister")
	}
	seen := map[string]bool{}
	for _, r := range c.Replies {
		if !audience[r.MinisterID] || seen[r.MinisterID] || strings.TrimSpace(r.Text) == "" || len(r.Text) > 40000 {
			return invalid("history invalid chat reply")
		}
		seen[r.MinisterID] = true
	}
	return nil
}
func inspectFacts(s State, target string) (map[string]any, error) {
	switch target {
	case "treasury":
		escrow, spent, grainEscrow := int64(0), int64(0), int64(0)
		for _, p := range s.Projects {
			spent += p.Spent
			if p.Reserved {
				escrow += p.Budget - p.Spent
				grainEscrow += p.Grain - p.GrainSpent
			}
		}
		return map[string]any{"turn": s.Turn, "treasury": s.Treasury, "privy_purse": s.PrivyPurse, "grain": s.Grain, "escrow": escrow, "spent": spent, "grain_escrow": grainEscrow}, nil
	case "regions":
		return map[string]any{"turn": s.Turn, "regions": s.Regions}, nil
	}
	if p := project(&s, target); p != nil {
		return map[string]any{"turn": s.Turn, "project": *p}, nil
	}
	for _, r := range s.Reports {
		if r.ID == target {
			return map[string]any{"report": r, "current_turn": s.Turn, "historical": true}, nil
		}
	}
	return nil, invalid("history inspection target not found")
}
