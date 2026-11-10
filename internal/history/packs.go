package history

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed scenarios/*.json
var scenarioAssets embed.FS

const MaxScenarioBytes = 2 * 1024 * 1024

func DecodeScenario(data []byte) (Scenario, error) {
	var p Scenario
	if len(data) > MaxScenarioBytes {
		return p, invalid("scenario exceeds 2 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return p, invalid("scenario JSON: %v", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return p, invalid("scenario has trailing data")
	}
	return p, ValidateScenario(p)
}

func ValidateScenario(p Scenario) error {
	if p.Schema != 1 || !safeID.MatchString(p.ID) || p.Version == "" || len(p.Version) > 40 || p.Name.ZH == "" || p.Name.EN == "" || len(p.Rules) == 0 || len(p.Rules) > 48000 || p.Calendar.Quarter < 1 || p.Calendar.Quarter > 4 {
		return invalid("scenario header is invalid")
	}
	if len(p.Initial.Entities) == 0 || len(p.Initial.Entities) > 512 || len(p.Ministers) > 160 || len(p.Metrics) > 80 || len(p.Resources) > 20 || len(p.Initial.Accounts) > 128 || len(p.Objectives) > 32 {
		return invalid("scenario capacity exceeded")
	}
	ids := map[string]bool{}
	for _, m := range p.Metrics {
		if !safeID.MatchString(m.ID) || ids[m.ID] || m.Min > m.Max || m.Min < -1000000000000 || m.Max > 1000000000000 {
			return invalid("invalid metric definition")
		}
		ids[m.ID] = true
	}
	ids = map[string]bool{}
	for _, r := range p.Resources {
		if !safeID.MatchString(r.ID) || ids[r.ID] || r.MaxTransaction < 1 || r.MaxTransaction > 1000000000000 {
			return invalid("invalid resource definition")
		}
		ids[r.ID] = true
	}
	ids = map[string]bool{}
	for _, m := range p.Ministers {
		if !safeID.MatchString(m.ID) || ids[m.ID] || m.Name == "" || m.NameEN == "" {
			return invalid("invalid minister definition")
		}
		ids[m.ID] = true
	}
	if len(p.Initial.Orders) > 0 || p.Initial.Ending != "" {
		return invalid("initial world must not contain runtime orders or ending")
	}
	if err := validateWorld(p, p.Initial); err != nil {
		return err
	}
	for _, m := range p.Ministers {
		e := entityByID(&p.Initial, m.ID)
		if e == nil || e.Kind != "person" {
			return invalid("minister requires a person entity")
		}
	}
	for _, g := range p.Objectives {
		e := entityByID(&p.Initial, g.EntityID)
		if !safeID.MatchString(g.ID) || e == nil || metricDef(p, g.Metric) == nil || (g.Operator != "gte" && g.Operator != "lte") || (g.Ending != "" && g.Ending != "victory" && g.Ending != "defeat") {
			return invalid("invalid objective")
		}
	}
	if p.PlayerEntityID != "" {
		entity := entityByID(&p.Initial, p.PlayerEntityID)
		if entity == nil || entity.Kind != "state" {
			return invalid("player entity must identify a state")
		}
	}
	return validatePresentation(p)
}

func (s *Store) Scenarios() ([]Scenario, error) {
	entries, err := scenarioAssets.ReadDir("scenarios")
	if err != nil {
		return nil, err
	}
	all := []Scenario{}
	seen := map[string]bool{}
	for _, e := range entries {
		b, err := scenarioAssets.ReadFile("scenarios/" + e.Name())
		if err != nil {
			return nil, err
		}
		p, err := DecodeScenario(b)
		if err != nil {
			return nil, fmt.Errorf("builtin %s: %w", e.Name(), err)
		}
		all = append(all, p)
		seen[p.ID] = true
	}
	files, err := os.ReadDir(filepath.Join(s.root, "scenarios"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		info, err := f.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > MaxScenarioBytes {
			return nil, invalid("invalid scenario file")
		}
		b, err := os.ReadFile(filepath.Join(s.root, "scenarios", f.Name()))
		if err != nil {
			return nil, err
		}
		p, err := DecodeScenario(b)
		if err != nil {
			return nil, fmt.Errorf("custom %s: %w", f.Name(), err)
		}
		if seen[p.ID] {
			return nil, invalid("duplicate scenario id")
		}
		all = append(all, p)
		seen[p.ID] = true
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all, nil
}

func (s *Store) Scenario(id string) (Scenario, error) {
	all, err := s.Scenarios()
	if err != nil {
		return Scenario{}, err
	}
	for _, p := range all {
		if p.ID == id {
			return p, nil
		}
	}
	return Scenario{}, ErrNotFound
}

// Import never overwrites a pack. Author revisions use a new ID, while existing
// saves retain their complete frozen package independently of the library.
func (s *Store) ImportScenario(data []byte) (Scenario, error) {
	p, err := DecodeScenario(data)
	if err != nil {
		return p, err
	}
	all, err := s.Scenarios()
	if err != nil {
		return p, err
	}
	for _, old := range all {
		if old.ID == p.ID {
			if digest(old) == digest(p) {
				return old, nil
			}
			return p, ErrConflict
		}
	}
	dir := filepath.Join(s.root, "scenarios")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return p, err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return p, err
	}
	tmp, err := os.CreateTemp(dir, ".import-*.tmp")
	if err != nil {
		return p, err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(b); err != nil {
		tmp.Close()
		return p, err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return p, err
	}
	if err = tmp.Close(); err != nil {
		return p, err
	}
	// Hard linking commits a complete file without replacing an existing ID.
	if err = os.Link(name, filepath.Join(dir, p.ID+".json")); err != nil {
		return p, err
	}
	return p, nil
}

func newScenarioState(id, name, locale string, p Scenario) State {
	w := p.Initial
	s := State{ID: id, Name: name, ScenarioID: p.ID, Scenario: &p, ScenarioDigest: digest(p), World: &w, Locale: locale, Year: p.Calendar.Year, Quarter: p.Calendar.Quarter, Ministers: p.Ministers, Regions: []Region{}}
	normalizeSlices(&s)
	result := cloneState(s)
	if strings.HasPrefix(strings.ToLower(locale), "en") {
		for i := range result.World.Events {
			event := &result.World.Events[i]
			if event.TitleEN != "" {
				event.Title = event.TitleEN
			}
			if event.TextEN != "" {
				event.Text = event.TextEN
			}
			if event.FollowUpEN != "" {
				event.FollowUp = event.FollowUpEN
			}
		}
	}
	return result
}

func metricDef(p Scenario, id string) *MetricDefinition {
	for i := range p.Metrics {
		if p.Metrics[i].ID == id {
			return &p.Metrics[i]
		}
	}
	return nil
}
func resourceDef(p Scenario, id string) *ResourceDefinition {
	for i := range p.Resources {
		if p.Resources[i].ID == id {
			return &p.Resources[i]
		}
	}
	return nil
}
func entityByID(w *World, id string) *Entity {
	for i := range w.Entities {
		if w.Entities[i].ID == id {
			return &w.Entities[i]
		}
	}
	return nil
}
func accountByID(w *World, id string) *Account {
	for i := range w.Accounts {
		if w.Accounts[i].ID == id {
			return &w.Accounts[i]
		}
	}
	return nil
}
func orderByID(w *World, id string) *Order {
	for i := range w.Orders {
		if w.Orders[i].ID == id {
			return &w.Orders[i]
		}
	}
	return nil
}

func validateWorld(p Scenario, w World) error {
	ids := map[string]bool{}
	for _, e := range w.Entities {
		if !safeID.MatchString(e.ID) || ids[e.ID] || e.Name.ZH == "" || e.Name.EN == "" || len(e.Attributes) > 32 || len(e.Metrics) > 80 {
			return invalid("invalid entity %s", e.ID)
		}
		ids[e.ID] = true
		for k, v := range e.Metrics {
			d := metricDef(p, k)
			if d == nil || v < d.Min || v > d.Max {
				return invalid("entity %s metric %s out of range", e.ID, k)
			}
		}
		for _, v := range e.Attributes {
			if len(v.ZH) > 4000 || len(v.EN) > 4000 {
				return invalid("entity text too long")
			}
		}
	}
	for _, e := range w.Entities {
		if (e.Location != "" && !ids[e.Location]) || (e.Owner != "" && !ids[e.Owner]) {
			return invalid("entity reference missing")
		}
	}
	audienceValid := func(audience []string) bool {
		for _, id := range audience {
			if id == "public" || id == "player" {
				continue
			}
			e := entityByID(&w, id)
			if e == nil || e.Kind != "person" {
				return false
			}
		}
		return true
	}
	for _, e := range w.Entities {
		if !audienceValid(e.Audience) {
			return invalid("entity audience references missing person")
		}
	}
	for _, e := range w.Events {
		if !audienceValid(e.Audience) {
			return invalid("event audience references missing person")
		}
	}
	accounts := map[string]bool{}
	for _, a := range w.Accounts {
		if !safeID.MatchString(a.ID) || accounts[a.ID] || !ids[a.Owner] || resourceDef(p, a.Resource) == nil || a.Balance < 0 || a.Balance > 1000000000000 || !audienceValid(a.Audience) {
			return invalid("invalid account %s", a.ID)
		}
		accounts[a.ID] = true
	}
	for _, o := range w.Orders {
		if (o.Disclosure != "" && o.Disclosure != "public" && o.Disclosure != "sealed") || !safeID.MatchString(o.ID) || o.Version < 1 || o.Progress < 0 || o.Progress > 100 {
			return invalid("invalid order")
		}
		for k, v := range o.Spent {
			if resourceDef(p, k) == nil || v < 0 {
				return invalid("invalid order cost")
			}
		}
	}
	return nil
}
