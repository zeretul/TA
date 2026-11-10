package app

import (
	"strings"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

func (a *App) localizeJourneyState(view JourneyState, context interactive.StoryContext, locale string) JourneyState {
	if !strings.HasPrefix(locale, "en") || context.Meta.PackBinding == nil {
		return view
	}
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()
	if cfg == nil {
		return view
	}
	binding := context.Meta.PackBinding
	pack, err := storypack.NewLibrary(cfg.DataDir()).Get(binding.PackID)
	if err != nil || pack.ContentHash() != binding.ContentHash {
		return view
	}
	return localizeJourneyPlaces(view, pack)
}

// Only translate known authored labels from the exact bound pack. IDs, route
// eligibility, durations and player-written location text remain unchanged.
func localizeJourneyPlaces(view JourneyState, pack storypack.Pack) JourneyState {
	names := map[string]string{}
	// These original route endpoints predate the pack's broader regional World rows.
	// Their registered endpoints are built-in labels, not player-written prose.
	if pack.ID == "fanren-jie" {
		names["黄枫谷山门内"] = "Inside Huangfeng Valley"
		names["黄枫谷对外入口"] = "Huangfeng Valley Entrance"
		names["公共坊市"] = "Public Market"
		names["越国坊市外围散修人家"] = "Cultivators' Homes outside Yue Market"
	}
	for _, place := range pack.World {
		if name := pack.Localizations["en-US"].World[place.ID].Name; name != "" {
			names[place.Name] = name
		}
	}
	word := func(value string) string {
		if name := names[value]; name != "" {
			return name
		}
		return value
	}
	view.Location = word(view.Location)
	view.Routes = append([]interactive.JourneyRouteOption(nil), view.Routes...)
	for i := range view.Routes {
		r := &view.Routes[i]
		r.OriginLabel, r.DestinationLabel = word(r.OriginLabel), word(r.DestinationLabel)
	}
	if view.Active != nil {
		active := *view.Active
		active.Origin, active.Destination = word(active.Origin), word(active.Destination)
		view.Active = &active
	}
	return view
}
