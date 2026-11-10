package history

import (
	"math"
	"regexp"
)

// Presentation is frozen author data, never simulation output. It contains
// decorative local assets and geographic anchors, not ownership or troop state.
type Presentation struct {
	Backdrop  string              `json:"backdrop,omitempty"`
	Portraits map[string]Portrait `json:"portraits,omitempty"`
	ViewBox   [4]float64          `json:"view_box"`
	Regions   []MapRegion         `json:"regions,omitempty"`
	Routes    [][2]string         `json:"routes,omitempty"`
}

type Portrait struct {
	Asset   string `json:"asset"`
	Index   int    `json:"index"`
	Columns int    `json:"columns"`
	Rows    int    `json:"rows"`
}

type MapRegion struct {
	Label    *Text          `json:"label,omitempty"`
	EntityID string         `json:"entity_id"`
	Center   [2]float64     `json:"center"`
	Rings    [][][2]float64 `json:"rings,omitempty"`
}

var historyAssetPath = regexp.MustCompile(`^(/history/[a-z0-9][a-z0-9_-]*|/api/history/assets/[a-f0-9]{64})\.(png|webp)$`)

func validatePresentation(s Scenario) error {
	p := s.Presentation
	if p == nil {
		return nil
	}
	for _, path := range []string{p.Backdrop} {
		if path != "" && !historyAssetPath.MatchString(path) {
			return invalid("invalid presentation asset")
		}
	}
	if len(p.Portraits) > 384 || len(p.Regions) > 128 || len(p.Routes) > 512 {
		return invalid("presentation capacity exceeded")
	}
	for id, portrait := range p.Portraits {
		e := entityByID(&s.Initial, id)
		if e == nil || e.Kind != "person" || !historyAssetPath.MatchString(portrait.Asset) || portrait.Columns < 1 || portrait.Columns > 16 || portrait.Rows < 1 || portrait.Rows > 16 || portrait.Index < 0 || portrait.Index >= portrait.Columns*portrait.Rows {
			return invalid("invalid portrait reference")
		}
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) <= 10000 }
	for _, v := range p.ViewBox {
		if !finite(v) {
			return invalid("invalid map bounds")
		}
	}
	if len(p.Regions) > 0 && (p.ViewBox[2] <= 0 || p.ViewBox[3] <= 0) {
		return invalid("invalid map dimensions")
	}
	seen := map[string]bool{}
	points := 0
	for _, r := range p.Regions {
		if entityByID(&s.Initial, r.EntityID) == nil || seen[r.EntityID] || !finite(r.Center[0]) || !finite(r.Center[1]) {
			return invalid("invalid map region")
		}
		seen[r.EntityID] = true
		for _, ring := range r.Rings {
			if len(ring) < 3 {
				return invalid("invalid map polygon")
			}
			points += len(ring)
			for _, pt := range ring {
				if !finite(pt[0]) || !finite(pt[1]) {
					return invalid("invalid map coordinate")
				}
			}
		}
	}
	if points > 24000 {
		return invalid("map point capacity exceeded")
	}
	for _, route := range p.Routes {
		if !seen[route[0]] || !seen[route[1]] || route[0] == route[1] {
			return invalid("invalid map route")
		}
	}
	return nil
}
