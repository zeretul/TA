package app

import (
	"fmt"
	"sort"
	"strings"

	"ta/internal/interactive"
	"ta/internal/media"
)

// PlacePictureImport is one picture the author picked for one place in one world.
//
// A place a picture may bind to is one the story actually knows: either a place
// the world pack's frozen catalog registered, or one this branch's own world
// record registered as the story played. A name that is neither is refused,
// because the picture would then be bound to something the game will never ask
// for while the manifest claimed it was attached.
//
// The request names the story and the place; the world comes from the story's
// pack binding, so a page cannot attach a picture to another world's place of
// the same name. A place the story invented is additionally bound to that story,
// so a sibling story that made up the same name keeps its own picture.
//
// Place may be the scene's location text rather than the record's own spelling.
// The location is prose the story wrote, so it is resolved to the place it
// names; a page never has to know the registered spelling.
type PlacePictureImport struct {
	StoryID string
	Place   string
	// Filename is the name the file arrived under. Only its extension is read,
	// and only after the bytes have been decoded.
	Filename string
	Data     []byte
}

// placeBinding is what a place picture binds to. Scope is empty for a place the
// world pack registered — every story in that world draws it — and names the
// story for a place that story invented.
type placeBinding struct {
	World string
	Place string
	Scope string
}

// PlacePictureState is one place's picture as the window would draw it.
//
// It is deliberately narrower than a person's picture: a place has no chosen
// avatar, no world-book picture and no shipped default, so the only question is
// whether an external picture answers. An empty URL means the window shows the
// place as text alone, which is what it already does.
type PlacePictureState struct {
	URL string `json:"url,omitempty"`
	// Overridden says whether an external picture stands and can be removed.
	Overridden bool `json:"overridden"`
	// Place is the registered place the picture belongs to, in the catalog's own
	// spelling. Empty means the scene's location names no registered place, so
	// there is nothing a picture could be attached to.
	Place string `json:"place,omitempty"`
}

// PlacePictureImportResult is what the page needs to draw the new picture.
type PlacePictureImportResult struct {
	Place   string            `json:"place"`
	World   string            `json:"world,omitempty"`
	Asset   media.Asset       `json:"asset"`
	URL     string            `json:"url"`
	Picture PlacePictureState `json:"picture"`
}

// PlacePictureImport copies one uploaded picture into the media directory and
// binds it to a place this story already knows.
func (a *App) PlacePictureImport(request PlacePictureImport) (PlacePictureImportResult, error) {
	target, err := a.placeTarget(request.StoryID, request.Place)
	if err != nil {
		return PlacePictureImportResult{}, err
	}
	library := a.Media()
	if library == nil {
		return PlacePictureImportResult{}, media.ErrNoMediaDir
	}
	asset, err := library.Import(media.ImportRequest{
		World:    target.World,
		Subject:  media.SubjectPlace,
		Scope:    target.Scope,
		Place:    target.Place,
		Purpose:  media.PurposeScene,
		Filename: request.Filename,
		Data:     request.Data,
	})
	if err != nil {
		return PlacePictureImportResult{}, err
	}
	return PlacePictureImportResult{
		Place:   target.Place,
		World:   target.World,
		Asset:   asset,
		URL:     media.URL(asset),
		Picture: PlacePictureState{URL: media.URL(asset), Overridden: true, Place: target.Place},
	}, nil
}

// PlacePictureClear drops this place's external picture.
func (a *App) PlacePictureClear(request PlacePictureImport) (PlacePictureImportResult, error) {
	target, err := a.placeTarget(request.StoryID, request.Place)
	if err != nil {
		return PlacePictureImportResult{}, err
	}
	library := a.Media()
	if library == nil {
		return PlacePictureImportResult{}, media.ErrNoMediaDir
	}
	result := PlacePictureImportResult{Place: target.Place, World: target.World}
	asset, ok := library.Picture(media.PictureQuery{
		World: target.World, Scope: target.Scope, Place: target.Place, Purpose: media.PurposeScene,
	})
	if !ok {
		// Nothing external answers, so there is nothing to clear. Reporting the
		// place as it already stands is more useful than an error.
		return result, nil
	}
	removed, err := library.Remove(asset.ID)
	if err != nil {
		return PlacePictureImportResult{}, err
	}
	result.Asset = removed
	return result, nil
}

// PlacePicture reports one place's picture as the window would draw it.
func (a *App) PlacePicture(storyID, place string) (PlacePictureState, error) {
	target, err := a.placeTarget(storyID, place)
	if err != nil {
		return PlacePictureState{}, err
	}
	state := PlacePictureState{Place: target.Place}
	library := a.Media()
	if library == nil {
		return state, nil
	}
	asset, ok := library.Picture(media.PictureQuery{
		World: target.World, Scope: target.Scope, Place: target.Place, Purpose: media.PurposeScene,
	})
	if !ok {
		return state, nil
	}
	state.URL = media.URL(asset)
	state.Overridden = true
	return state, nil
}

// placeTarget resolves a place to the world, the story that owns it, and the
// registered spelling.
//
// Two sources count as knowing a place, in this order:
//
//  1. the world pack's frozen catalog, which is what the story started with;
//  2. this branch's own world record, which is what the story registered as it
//     played. A place the narrator invented and the rules then recorded is a real
//     place in this save, and refusing its picture would make the library offer a
//     setting that always fails.
//
// Catalog places come first so a story that later registers a same-named record
// keeps the world's shared picture rather than forking it.
//
// The name may be either spelling or the scene's location prose. A name that
// resolves to neither source is refused: accepting any string would let a page
// bind a picture to a place that exists nowhere, and the picture would then never
// be drawn while the manifest claimed it was attached.
func (a *App) placeTarget(storyID, place string) (placeBinding, error) {
	if a == nil {
		return placeBinding{}, ErrNoWorkspace
	}
	name := strings.Join(strings.Fields(place), " ")
	if name == "" {
		return placeBinding{}, fmt.Errorf("a picture belongs to a place")
	}
	a.mu.RLock()
	store := a.interactive
	a.mu.RUnlock()
	if store == nil {
		return placeBinding{}, ErrNoWorkspace
	}
	storyID = strings.TrimSpace(storyID)
	storyContext, err := store.StoryContext(storyID, "")
	if err != nil {
		return placeBinding{}, err
	}
	world := ""
	if binding := storyContext.Meta.PackBinding; binding != nil {
		world = strings.TrimSpace(binding.PackID)
	}
	// Longest first, so a nested name resolves to the more specific place: a
	// world that registers both 坊市 and 越国西境坊市 means the latter when the
	// story says the player is in 越国西境坊市.
	for _, ref := range placeRefs(storyID, storyContext.Snapshot.State, storyContext.Meta.PackCatalog) {
		if strings.Contains(name, ref.Name) {
			return placeBinding{World: world, Place: ref.Name, Scope: ref.Scope}, nil
		}
	}
	return placeBinding{}, fmt.Errorf("this story does not know a place called %q", name)
}

// placeRef is one place a window may draw: the registered spelling and the
// story that owns it. Scope is empty for a place the world pack registered and
// names the story for a place that story invented.
type placeRef struct {
	Name  string
	Scope string
}

// placeRefs lists every place this branch may draw, longest first.
//
// Two sources count, and both are places the story actually knows: the world
// pack's frozen catalog, which is what the story started with, and this branch's
// own world record, which is what the story registered as it played. Catalog
// names come first so a story that later records a same-named place keeps the
// world's shared picture instead of forking it.
func placeRefs(storyID string, state map[string]any, catalog *interactive.PackCatalogSnapshot) []placeRef {
	catalogNames := packPlaceNames(catalog)
	refs := make([]placeRef, 0, len(catalogNames))
	seen := make(map[string]struct{}, len(catalogNames))
	for _, name := range catalogNames {
		seen[name] = struct{}{}
		refs = append(refs, placeRef{Name: name})
	}
	scope := strings.TrimSpace(storyID)
	for _, name := range localPlaceNames(state) {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		refs = append(refs, placeRef{Name: name, Scope: scope})
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if len(refs[i].Name) != len(refs[j].Name) {
			return len(refs[i].Name) > len(refs[j].Name)
		}
		return refs[i].Name < refs[j].Name
	})
	return refs
}

// localPlaceNames lists the place records this branch registered, longest first
// for the same reason the catalog list is ordered that way.
func localPlaceNames(state map[string]any) []string {
	actors, _ := state[actorStateRoot].(map[string]any)
	world, _ := actors[interactive.DefaultWorldEntitiesActorID].(map[string]any)
	fields, _ := world["state"].(map[string]any)
	records, _ := fields[localPlaceRecordsField].(map[string]any)
	if len(records) == 0 {
		return nil
	}
	names := make([]string, 0, len(records))
	for key, value := range records {
		if _, ok := value.(map[string]any); !ok {
			continue
		}
		registered := strings.Join(strings.Fields(key), " ")
		if registered == "" {
			continue
		}
		names = append(names, registered)
	}
	sort.SliceStable(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j])
		}
		return names[i] < names[j]
	})
	return names
}

// localPlaceRecordsField is the world Actor's field holding the places this
// branch registered. It is the same field the 本局新建 page reads.
const localPlaceRecordsField = "地点记录"
