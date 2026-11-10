package api

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"ta/config"
	runtimeapp "ta/internal/app"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
	"ta/internal/storypack"
)

type reviewedPortraitNativePack struct {
	id, world, hash string
	names           map[string]string
}

type reviewedPortraitHTTPScenario struct {
	location     string
	allow        bool
	rejectCommit bool
}

// Read the actual compiled-in pack contents against an empty data root. An
// unauthorized test-only manifest cannot turn an arbitrary string into a hash.
func reviewedPortraitNativePacks(t *testing.T, manifest media.StoryManifest) []reviewedPortraitNativePack {
	t.Helper()
	for _, id := range []string{"harry-potter-1991-ta07-full-gm-candidate", "harry-potter-1991-student-ta07-full-gm-candidate"} {
		if manifest.PackWorlds[id] != "harry_potter" {
			t.Fatalf("both authored Harry Potter configurations are required: %s", id)
		}
	}
	ids := make([]string, 0, len(manifest.PackWorlds))
	for id := range manifest.PackWorlds {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	library := storypack.NewLibrary(t.TempDir())
	packs := make([]reviewedPortraitNativePack, 0, len(ids))
	for _, id := range ids {
		content, err := library.GetContent(id)
		if err != nil {
			t.Fatal(err)
		}
		hash := content.ContentHash()
		allowed := false
		for _, accepted := range manifest.PackHashes[id] {
			if accepted == hash && hash != "" {
				allowed = true
			}
		}
		if !allowed {
			t.Fatalf("manifest does not authorize the actual native content hash for %s; the pure-gate fixture is not an API installation", id)
		}
		names := map[string]string{}
		for _, person := range content.Characters {
			if person.RecordOnly {
				continue
			}
			if _, duplicate := names[person.SourceKey]; duplicate {
				t.Fatalf("duplicate active native catalog source key in %s: %s", id, person.SourceKey)
			}
			names[person.SourceKey] = person.Name
		}
		packs = append(packs, reviewedPortraitNativePack{id: id, world: manifest.PackWorlds[id], hash: hash, names: names})
	}
	return packs
}

func reviewedPortraitColdApp(t *testing.T, root, dir string) (*runtimeapp.App, *Server) {
	t.Helper()
	application, err := runtimeapp.New(t.Context(), &config.Config{OpenAIModel: "test", NovaDir: root, Workspace: root, ResumeLastWorkspace: false, OpenAIBaseURL: "http://127.0.0.1:1/v1", MediaDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	return application, NewServer(application, "0")
}

// These assertions are shared by allowed and denied nonempty-location cases.
// A negative must not pass merely because its formal identity/source is absent
// or its visual snapshot differs from its committed scene.
func assertReviewedPortraitFormalObservation(t *testing.T, asset media.StoryAsset, ctx interactive.StoryContext, state character.State, knownAs, location, turnID string) {
	t.Helper()
	if len(state.Knowledge) != 1 || len(state.Exchanges) != 1 || state.Knowledge[0].Visual == nil {
		t.Fatal("expected one complete formally submitted visual and self-introduction")
	}
	p, ok := state.Profile("observed")
	if !ok || p.StoryID != ctx.Snapshot.StoryID || p.SourceKey != asset.SourceKey || p.Name != asset.Rule.CanonicalName || p.KnownAs != knownAs || !asset.Rule.AllowsKnownAs(p.KnownAs) || (strings.TrimSpace(p.Role) == "" || (asset.Rule.RoleSemantics != "background_classification" && p.Role != asset.Rule.Role)) || p.Lifeform != asset.Rule.Lifeform || p.Gender != asset.Rule.Gender {
		t.Fatal("formal subject identity/profile did not match the reviewed mapping")
	}
	k, e := state.Knowledge[0], state.Exchanges[0]
	v := k.Visual
	scene := interactive.StoryScene(ctx.Snapshot)
	if location == "" || scene.Location != location || v.Location != location || !k.Live() || k.Nature != character.NatureExperienced || k.CharacterID != "protagonist" || !character.HolderCanKnow(k, "protagonist") || k.FormalCommitID == "" || k.SourceEventID != k.FormalCommitID || k.SourceTurnID != turnID || k.SourceExchangeID != e.ID {
		t.Fatal("formal observation location/provenance is incomplete or inconsistent")
	}
	if v.SubjectID != p.ID || v.SourceKey != p.SourceKey || v.KnownAs != p.KnownAs || v.StoryID != ctx.Snapshot.StoryID || v.BranchID != "main" || v.ContactExchangeID != e.ID || v.RecognitionExchangeID != e.ID || !character.VisualRoleSnapshotMatches(*v, p) || v.Appearance != interactive.StoryVisualAppearance(ctx.Snapshot.State, p.ID) || v.Form != asset.Rule.Form || !character.VisualAppearanceSupports(v.Appearance, v.Form) || v.Lifeform != p.Lifeform || v.Gender != p.Gender || v.Age != p.Age || v.Year != character.ExplicitVisualYear(scene.WorldTime) {
		t.Fatal("formal visual snapshot drifted from the actual subject or scene")
	}
	if !character.ContainsID(scene.Present, "protagonist") || !character.ContainsID(scene.Present, p.ID) || e.StoryID != v.StoryID || e.BranchID != v.BranchID || e.Channel != character.ChannelScene || e.TurnID != turnID || e.SourceEventID != k.FormalCommitID || e.WorldTime != k.WorldTime || k.WorldTime != scene.WorldTime || !character.ContainsID(e.Audience, "protagonist") || !character.ContainsID(e.Audience, p.ID) || !character.VisualIdentitySupports(e, p, "protagonist") {
		t.Fatal("formal source is not the witnessed current-scene self-introduction")
	}
	if asset.Rule.RoleSemantics != "background_classification" {
		recognitionProfile := p
		if asset.Rule.RoleRecognition != "" {
			recognitionProfile.Role = asset.Rule.RoleRecognition
		}
		if !character.VisualRecognitionSupports(e, recognitionProfile, "protagonist") {
			t.Fatal("formal self-introduction did not establish the required public role")
		}
	}
}

func assertReviewedPortraitHTTPDenied(t *testing.T, application *runtimeapp.App, server *Server, asset media.StoryAsset, ctx interactive.StoryContext) {
	t.Helper()
	q := url.Values{"branch": {ctx.Snapshot.BranchID}, "head": {interactive.StoryMediaHead(ctx.Snapshot)}, "revision": {fmt.Sprint(ctx.Snapshot.ContextRevision)}, "actor": {"observed"}}
	target := "/api/interactive/stories/" + ctx.Snapshot.StoryID + "/media/" + asset.ID + "?" + q.Encode()
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, rangeValue := range []string{"", "bytes=0-7"} {
			var headers []ut.Header
			if rangeValue != "" {
				headers = append(headers, ut.Header{Key: "Range", Value: rangeValue})
			}
			response := ut.PerformRequest(server.engine.Engine, method, target, nil, headers...)
			if response.Code != 404 || response.Body.Len() != 0 {
				t.Fatalf("location/source restriction leaked %s range=%q: %d", method, rangeValue, response.Code)
			}
		}
	}
	window, err := application.CharacterWindow(runtimeapp.CharacterWindowRequest{StoryID: ctx.Snapshot.StoryID, BranchID: ctx.Snapshot.BranchID})
	if err != nil {
		t.Fatal(err)
	}
	for _, person := range window.Characters {
		if person.ID == "observed" && (person.Avatar != "" || person.AvatarDisplay != "" || person.AvatarLorePath != "" || person.ImageCaption != "") {
			t.Fatalf("location/source restriction exposed image metadata or fallback: %+v", person)
		}
	}
}
