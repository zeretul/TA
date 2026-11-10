package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	runtimeapp "ta/internal/app"
	"ta/internal/character"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/media"
)

// TestAllReviewedPortraitsFormalCommitColdHTTP uses actual read-only image bytes
// and explicitly synthetic, isolated story facts. It is NOT a natural model test.
// All application/store writes are confined to t.TempDir. No image is copied,
// linked, renamed, rewritten or revoked in the supplied installation.
func TestAllReviewedPortraitsFormalCommitColdHTTP(t *testing.T) {
	dir := os.Getenv("TA_STORY_MEDIA_REAL_DIR")
	if dir == "" {
		t.Skip("set TA_STORY_MEDIA_REAL_DIR to the full reviewed media installation")
	}
	manifestPath := filepath.Join(dir, "story-assets.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifestHash := sha256.Sum256(raw)
	t.Cleanup(func() {
		after, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Error(err)
			return
		}
		if sha256.Sum256(after) != manifestHash {
			t.Error("read-only manifest changed")
		}
	})
	var manifest media.StoryManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Assets) != 843 {
		t.Fatalf("expected full 843-asset manifest, got %d", len(manifest.Assets))
	}
	var reviewed []media.StoryAsset
	for _, asset := range manifest.Assets {
		if asset.Reviewed && asset.Rule != nil {
			reviewed = append(reviewed, asset)
		}
	}
	if len(reviewed) != 99 {
		t.Fatalf("expected 99 reviewed rule-bound portraits, got %d", len(reviewed))
	}
	sort.Slice(reviewed, func(i, j int) bool { return reviewed[i].ID < reviewed[j].ID })
	// Native hashes are obtained from the actual loaded built-ins, never from
	// a JSON-file SHA or the deliberately unauthorized pure-gate fixture.
	packs := reviewedPortraitNativePacks(t, manifest)
	seen := map[string]bool{}
	for _, asset := range reviewed {
		if seen[asset.ID] {
			t.Fatalf("duplicate reviewed asset ID: %s", asset.ID)
		}
		seen[asset.ID] = true
		t.Run(asset.ID, func(t *testing.T) {
			location := "隔离验收会面地点"
			switch asset.Rule.Kind {
			case "observed_named_portrait":
			case "observed_named_portrait_at_location":
				if asset.Rule.Location == "" {
					t.Fatal("location-bound portrait has no authored location")
				}
				location = asset.Rule.Location
			default:
				t.Fatalf("unsupported rule kind: %q", asset.Rule.Kind)
			}
			matchedPacks := 0
			for _, pack := range packs {
				if pack.world != asset.World {
					continue
				}
				matchedPacks++
				if pack.names[asset.SourceKey] != asset.Rule.CanonicalName {
					t.Fatalf("portrait identity is not an active native catalog member in %s", pack.id)
				}
				t.Run(pack.id, func(t *testing.T) {
					positive := reviewedPortraitHTTPScenario{location: location, allow: true}
					t.Run("canonical", func(t *testing.T) { runReviewedPortraitColdHTTP(t, dir, pack, asset, asset.Rule.KnownAs, positive) })
					for _, alias := range asset.Rule.KnownAsAliases {
						t.Run("public_title_"+alias, func(t *testing.T) { runReviewedPortraitColdHTTP(t, dir, pack, asset, alias, positive) })
					}
					if asset.Rule.Kind == "observed_named_portrait_at_location" {
						for _, tc := range []struct{ name, location string }{
							{"wrong_location", "隔离验收错误会面地点"},
							{"nearby", location + "附近"},
							{"not_arrived", "尚未到达" + location},
						} {
							t.Run(tc.name, func(t *testing.T) {
								runReviewedPortraitColdHTTP(t, dir, pack, asset, asset.Rule.KnownAs, reviewedPortraitHTTPScenario{location: tc.location})
							})
						}
						t.Run("empty_scene_commit_rejected", func(t *testing.T) {
							runReviewedPortraitColdHTTP(t, dir, pack, asset, asset.Rule.KnownAs, reviewedPortraitHTTPScenario{rejectCommit: true})
						})
					}
				})
			}
			if matchedPacks == 0 {
				t.Fatal("no matching native pack was exercised")
			}
		})
	}
}

func runReviewedPortraitColdHTTP(t *testing.T, dir string, pack reviewedPortraitNativePack, asset media.StoryAsset, knownAs string, scenario reviewedPortraitHTTPScenario) {
	t.Helper()
	if asset.File == "" || filepath.Base(asset.File) != asset.File || strings.ContainsAny(asset.File, `/\`) {
		t.Fatal("asset file is not a local basename")
	}
	sourcePath := filepath.Join(dir, asset.File)
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(sourceBytes)
	if hex.EncodeToString(sum[:]) != asset.SHA256 || int64(len(sourceBytes)) != asset.Bytes || len(sourceBytes) < 8 {
		t.Fatal("source image hash/size mismatch")
	}
	t.Cleanup(func() {
		after, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Error(err)
			return
		}
		if sha256.Sum256(after) != sum || len(after) != len(sourceBytes) {
			t.Error("read-only original image changed")
		}
	})
	root := t.TempDir()
	worldTime := fmt.Sprintf("%d年9月1日", asset.Rule.Year)
	if asset.Rule.TimeSemantics == "current_observation" {
		worldTime = "本局第1日清晨"
		if asset.World == "lotm" {
			worldTime = "1349年9月1日"
		}
	} else if asset.Rule.Year <= 0 {
		t.Fatal("explicit-year rule has no year")
	}
	appearance := asset.Rule.Form + "，正常衣着"
	spoken := "我叫" + knownAs + "。"
	if asset.Rule.RoleSemantics != "background_classification" {
		title := asset.Rule.Role
		if asset.Rule.RoleRecognition != "" {
			title = asset.Rule.RoleRecognition
		}
		spoken = "我是" + knownAs + "，" + title + "。"
	}
	if pack.world != asset.World || pack.id == "" || pack.hash == "" {
		t.Fatal("helper requires an explicit matching native pack")
	}
	packID, packHash := pack.id, pack.hash
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "媒体门禁确定性验收", PackBinding: &interactive.StoryPackBinding{PackID: packID, ContentHash: packHash, Scope: "media-test"}, PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: asset.SourceKey, Name: asset.Rule.CanonicalName, Role: asset.Rule.Role, Gender: asset.Rule.Gender, Lifeform: asset.Rule.Lifeform}}}, InitialStateOps: []interactivestate.Op{
		{Op: "set", Path: "actors.story.id", Value: "story"}, {Op: "set", Path: "actors.story.state.当前时间", Value: worldTime}, {Op: "set", Path: "actors.story.state.当前详细地点", Value: scenario.location}, {Op: "set", Path: "actors.story.state.在场角色", Value: []any{"protagonist", "observed"}},
		{Op: "set", Path: "actors.observed.id", Value: "observed"}, {Op: "set", Path: "actors.observed.name", Value: knownAs}, {Op: "set", Path: "actors.observed.state.外貌与装扮", Value: appearance},
	}})
	if err != nil {
		t.Fatal(err)
	}
	beforeTurn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "看看周围", Narrative: "你尚未与此人交谈或认识对方。"})
	if err != nil {
		t.Fatal(err)
	}
	beforeBranch, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: beforeTurn.ID, Title: "接触前分支"})
	if err != nil {
		t.Fatal(err)
	}
	beforeAttempt, beforeState, err := store.StoryMediaSnapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "向眼前人物问好并询问称呼", Narrative: "你当面看清此人的外貌，听见对方介绍自己。", TurnResult: &interactive.TurnResult{Choices: []string{"继续交谈", "观察环境", "询问近况", "安静等待", "告别离开"}, CharacterFacts: &interactive.TurnCharacterFactsInput{
		Profiles:  []interactive.TurnCharacterProfileInput{{CharacterID: "observed", Name: asset.Rule.CanonicalName, KnownAs: knownAs, SourceKey: asset.SourceKey, Role: asset.Rule.Role, Lifeform: asset.Rule.Lifeform, Gender: asset.Rule.Gender, Description: appearance}},
		Exchanges: []interactive.TurnCharacterExchangeInput{{CharacterID: "observed", Speaker: "character", Channel: "scene", AddressedTo: "protagonist", Text: spoken}},
		Knowledge: []interactive.TurnCharacterKnowledgeInput{{CharacterID: "protagonist", Statement: "亲眼看清此人当前形态，并听见对方介绍姓名。", Nature: "experienced", Channel: "scene", Visual: &character.VisualObservationDraft{SubjectID: "observed", Form: asset.Rule.Form, ContactExchangeIndex: 1, RecognitionExchangeIndex: 1}}},
	}}})
	if scenario.rejectCommit {
		// A blank committed scene cannot legitimately create a visual record.
		// Require the actual source validator's error, no half-written facts,
		// and a cold HTTP denial instead of fabricating a blank visual snapshot.
		if scenario.location != "" || err == nil || !strings.Contains(err.Error(), "visual knowledge lacks observed appearance, known identity or co-presence") {
			t.Fatalf("empty scene did not fail at the visual source validator: %v", err)
		}
		failedContext, failedState, readErr := store.StoryMediaSnapshot(story.ID, "main")
		if readErr != nil {
			t.Fatal(readErr)
		}
		beforeJSON, beforeErr := json.Marshal(beforeState)
		afterJSON, afterErr := json.Marshal(failedState)
		if beforeErr != nil || afterErr != nil || !bytes.Equal(beforeJSON, afterJSON) || interactive.StoryMediaHead(failedContext.Snapshot) != interactive.StoryMediaHead(beforeAttempt.Snapshot) || failedContext.Snapshot.ContextRevision != beforeAttempt.Snapshot.ContextRevision || interactive.StoryScene(failedContext.Snapshot).Location != "" {
			t.Fatal("rejected empty-location observation changed committed facts")
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		application, server := reviewedPortraitColdApp(t, root, dir)
		assertReviewedPortraitHTTPDenied(t, application, server, asset, failedContext)
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	ctx, state, err := store.StoryMediaSnapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Knowledge) != 1 || state.Knowledge[0].Visual == nil {
		t.Fatal("formal tool facts did not persist observation")
	}
	if len(state.Exchanges) != 1 || state.Knowledge[0].Visual.ContactExchangeID != state.Knowledge[0].Visual.RecognitionExchangeID {
		t.Fatal("single formal self-introduction was not retained as both sources")
	}
	if state.Knowledge[0].WorldTime != worldTime {
		t.Fatal("knowledge clock differs from formal scene")
	}
	for _, exchange := range state.Exchanges {
		if exchange.WorldTime != worldTime {
			t.Fatal("exchange clock differs from formal scene")
		}
	}
	if state.Knowledge[0].Visual.Year != character.ExplicitVisualYear(worldTime) {
		t.Fatal("observation fabricated a calendar year")
	}
	assertReviewedPortraitFormalObservation(t, asset, ctx, state, knownAs, scenario.location, turn.ID)
	afterBranch, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: turn.ID, Title: "接触后分支"})
	if err != nil {
		t.Fatal(err)
	}
	beforeContext, _, err := store.StoryMediaSnapshot(story.ID, beforeBranch.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterContext, afterState, err := store.StoryMediaSnapshot(story.ID, afterBranch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterState.Knowledge) != 1 || afterState.Knowledge[0].Visual.BranchID != "main" {
		t.Fatal("fork rewrote historical source branch")
	}
	ctx, _, err = store.StoryMediaSnapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	application, server := reviewedPortraitColdApp(t, root, dir)
	if !scenario.allow {
		// The persisted observation was checked above: it is complete and has
		// exactly the same location as the scene, but violates the authored place.
		assertReviewedPortraitHTTPDenied(t, application, server, asset, ctx)
		assertReviewedPortraitHTTPDenied(t, application, server, asset, afterContext)
		return
	}
	q := url.Values{"branch": {"main"}, "head": {interactive.StoryMediaHead(ctx.Snapshot)}, "revision": {fmt.Sprint(ctx.Snapshot.ContextRevision)}, "actor": {"observed"}}
	target := "/api/interactive/stories/" + story.ID + "/media/" + asset.ID + "?" + q.Encode()
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		result := ut.PerformRequest(server.engine.Engine, method, target, nil)
		if result.Code != 200 {
			t.Fatalf("%s positive: %d %s", method, result.Code, result.Body.String())
		}
		if got := result.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
			t.Fatalf("private image cache: %q", got)
		}
		if method == http.MethodGet {
			got := sha256.Sum256(result.Body.Bytes())
			if hex.EncodeToString(got[:]) != asset.SHA256 || int64(result.Body.Len()) != asset.Bytes || !bytes.Equal(result.Body.Bytes(), sourceBytes) {
				t.Fatal("real HTTP bytes differ from reviewed SHA")
			}
		}
	}
	ranged := ut.PerformRequest(server.engine.Engine, http.MethodGet, target, nil, ut.Header{Key: "Range", Value: "bytes=0-7"})
	if ranged.Code != 206 || !bytes.Equal(ranged.Body.Bytes(), sourceBytes[:8]) || ranged.Header().Get("Content-Range") != fmt.Sprintf("bytes 0-7/%d", asset.Bytes) {
		t.Fatalf("Range=%d bytes=%d", ranged.Code, ranged.Body.Len())
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, query := range []string{"", strings.Replace(q.Encode(), "branch=main", "branch=other", 1), strings.Replace(q.Encode(), "head="+url.QueryEscape(interactive.StoryMediaHead(ctx.Snapshot)), "head=stale", 1), strings.Replace(q.Encode(), "revision="+fmt.Sprint(ctx.Snapshot.ContextRevision), "revision=0", 1), strings.Replace(q.Encode(), "actor=observed", "actor=unknown", 1)} {
			bad := "/api/interactive/stories/" + story.ID + "/media/" + asset.ID + "?" + query
			result := ut.PerformRequest(server.engine.Engine, method, bad, nil, ut.Header{Key: "Range", Value: "bytes=0-7"})
			if result.Code != 404 || result.Body.Len() != 0 {
				t.Fatalf("unsafe URL %s: %d", bad, result.Code)
			}
		}
	}
	branchTarget := func(context interactive.StoryContext) string {
		values := url.Values{"branch": {context.Snapshot.BranchID}, "head": {interactive.StoryMediaHead(context.Snapshot)}, "revision": {fmt.Sprint(context.Snapshot.ContextRevision)}, "actor": {"observed"}}
		return "/api/interactive/stories/" + story.ID + "/media/" + asset.ID + "?" + values.Encode()
	}
	for _, test := range []struct {
		context interactive.StoryContext
		want    int
	}{{beforeContext, 404}, {afterContext, 200}} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			result := ut.PerformRequest(server.engine.Engine, method, branchTarget(test.context), nil)
			if result.Code != test.want {
				t.Fatalf("fork %s %s=%d want%d", test.context.Snapshot.BranchID, method, result.Code, test.want)
			}
		}
	}
	// Public aliases must not bypass protection. No alias file is created.
	for _, ext := range []string{filepath.Ext(asset.File), ".png", ".jpg", ".jpeg", ".webp"} {
		alias := strings.TrimSuffix(asset.File, filepath.Ext(asset.File)) + ext
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			response := ut.PerformRequest(server.engine.Engine, method, "/api/media/"+alias, nil, ut.Header{Key: "Range", Value: "bytes=0-7"})
			if response.Code != 404 {
				t.Fatalf("public alias %s %s=%d", method, alias, response.Code)
			}
		}
	}
	window, err := application.CharacterWindow(runtimeapp.CharacterWindowRequest{StoryID: story.ID, BranchID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range window.Characters {
		if p.ID == "observed" {
			found = true
			if knownAs != asset.Rule.KnownAs && strings.Contains(p.ImageCaption, asset.Rule.CanonicalName) {
				t.Fatal("public title caption reveals an unintroduced canonical name")
			}
			if p.Avatar != "" || p.AvatarDisplay != target || p.ImageCaption == "" {
				t.Fatalf("explicit-open display mismatch: %+v", p)
			}
		}
	}
	if !found {
		t.Fatal("missing picture consumer")
	}
	// A rewind removes the source facts and invalidates the old URL, including
	// HEAD/Range. Reading through a new store exercises journal refresh fencing.
	reopened := interactive.NewStore(root)
	defer reopened.Close()
	_, _, err = reopened.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "核对刚才的观察", Narrative: "你确认先前观察有误，撤回那条知识。", TurnResult: &interactive.TurnResult{Choices: []string{"继续交谈", "观察环境", "询问依据", "安静等待", "告别离开"}, CharacterFacts: &interactive.TurnCharacterFactsInput{Knowledge: []interactive.TurnCharacterKnowledgeInput{{CharacterID: "protagonist", Statement: "先前观察有误，撤回。", Nature: "experienced", Channel: "scene", Retracts: state.Knowledge[0].ID}}}}})
	if err != nil {
		t.Fatal(err)
	}
	retractedWindow, err := application.CharacterWindow(runtimeapp.CharacterWindowRequest{StoryID: story.ID, BranchID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range retractedWindow.Characters {
		if p.ID == "observed" && p.AvatarDisplay != "" {
			t.Fatal("formal retraction still displays picture")
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, rangeValue := range []string{"", "bytes=0-7"} {
			response := ut.PerformRequest(server.engine.Engine, method, target, nil, ut.Header{Key: "Range", Value: rangeValue})
			if response.Code != 404 || response.Body.Len() != 0 {
				t.Fatalf("formal retraction leaked %s range=%q: %d", method, rangeValue, response.Code)
			}
		}
	}
	retractedContext, _, err := reopened.StoryMediaSnapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		response := ut.PerformRequest(server.engine.Engine, method, branchTarget(retractedContext), nil, ut.Header{Key: "Range", Value: "bytes=0-7"})
		if response.Code != 404 || response.Body.Len() != 0 {
			t.Fatalf("fresh revoked context leaked %s: %d", method, response.Code)
		}
	}
	if err := reopened.RewindToTurnParent(story.ID, interactive.RewindTurnRequest{BranchID: "main", TurnID: turn.ID}); err != nil {
		t.Fatal(err)
	}
	stale := ut.PerformRequest(server.engine.Engine, http.MethodGet, target, nil, ut.Header{Key: "Range", Value: "bytes=0-7"})
	if stale.Code != 404 {
		t.Fatalf("rewound source leaked=%d", stale.Code)
	}
	inherited := ut.PerformRequest(server.engine.Engine, http.MethodGet, branchTarget(afterContext), nil)
	if inherited.Code != 200 {
		t.Fatalf("main rewind erased valid fork inheritance: %d", inherited.Code)
	}
}
