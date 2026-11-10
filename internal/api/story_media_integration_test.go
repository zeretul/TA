package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"ta/config"
	runtimeapp "ta/internal/app"
	"ta/internal/character"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/media"
)

// This fixture commits through the real formal-turn path, closes the store,
// and serves from a cold App. It is deterministic game-state QA, NOT evidence
// that a live model spontaneously produced a correct observation.
func TestStoryMediaFormalCommitColdReadHTTP(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "media")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("synthetic-test-image-bytes-not-pixel-evidence")
	sum := sha256.Sum256(payload)
	sha := hex.EncodeToString(sum[:])
	file := sha + ".png"
	const world = "test-world"
	const hash = "test-native-content-hash"
	asset := media.StoryAsset{ID: "observed-image", World: world, SourceKey: "hp-npc-sinistra", File: file, SHA256: sha, Bytes: int64(len(payload)), Reviewed: true, Rule: &media.StoryRule{Kind: "observed_named_portrait", Year: 1991, Form: "人类女性", Lifeform: "human", Gender: "female", KnownAs: "奥罗拉·辛尼斯塔", CanonicalName: "奥罗拉·辛尼斯塔", Role: "天文学教授", AdultIdentity: true}, Caption: "人物参考图（视觉细节为原创演绎）"}
	manifest := media.StoryManifest{Schema: "ta.story-media/v1", PackHashes: map[string][]string{world: {hash}}, PackWorlds: map[string]string{world: world}, Assets: []media.StoryAsset{asset}}
	if err := os.WriteFile(filepath.Join(dir, file), payload, 0600); err != nil {
		t.Fatal(err)
	}
	// Optional real source smoke uses installed, verified hardlinks without
	// copying original pixels. It still uses simulated committed story facts.
	if realDir := os.Getenv("TA_STORY_MEDIA_REAL_DIR"); realDir != "" {
		raw, err := os.ReadFile(filepath.Join(realDir, "story-assets.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &manifest); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, a := range manifest.Assets {
			if a.ID == "p11o-art-hp-person-sinistra-v1" {
				asset = a
				found = true
			}
		}
		if !found || !asset.Reviewed || asset.Rule == nil {
			t.Fatal("real Sinistra asset is not reviewed and rule-bound")
		}
		dir = realDir
		payload = nil
	}
	raw, _ := json.Marshal(manifest)
	if payload != nil {
		if err := os.WriteFile(filepath.Join(dir, "story-assets.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	packID, packHash := "", ""
	for id, w := range manifest.PackWorlds {
		if w == asset.World && len(manifest.PackHashes[id]) > 0 {
			packID = id
			packHash = manifest.PackHashes[id][0]
			break
		}
	}
	if packID == "" {
		t.Fatal("native matching pack hash missing")
	}
	store := interactive.NewStore(root)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "媒体门禁确定性验收", PackBinding: &interactive.StoryPackBinding{PackID: packID, ContentHash: packHash, Scope: "media-test"}, PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: asset.SourceKey, Name: asset.Rule.CanonicalName, Role: asset.Rule.Role, Gender: "female", Lifeform: "human"}}}, InitialStateOps: []interactivestate.Op{
		{Op: "set", Path: "actors.story.id", Value: "story"}, {Op: "set", Path: "actors.story.state.当前时间", Value: "1991年9月1日"}, {Op: "set", Path: "actors.story.state.当前详细地点", Value: "天文塔"}, {Op: "set", Path: "actors.story.state.在场角色", Value: []any{"protagonist", "sinistra"}},
		{Op: "set", Path: "actors.sinistra.id", Value: "sinistra"}, {Op: "set", Path: "actors.sinistra.name", Value: asset.Rule.KnownAs}, {Op: "set", Path: "actors.sinistra.state.外貌与装扮", Value: "人类女性，穿深色教师长袍"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	beforeTurn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "看看周围", Narrative: "你尚未与她交谈或认识她。"})
	if err != nil {
		t.Fatal(err)
	}
	beforeBranch, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: beforeTurn.ID, Title: "接触前分支"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "向眼前教授问好并询问称呼", Narrative: "你当面看清她的外貌，听见她向你介绍自己。", TurnResult: &interactive.TurnResult{Choices: []string{"继续交谈", "观察星空", "询问课程", "安静等待", "告别离开"}, CharacterFacts: &interactive.TurnCharacterFactsInput{
		Profiles:  []interactive.TurnCharacterProfileInput{{CharacterID: "sinistra", Name: asset.Rule.CanonicalName, KnownAs: asset.Rule.KnownAs, SourceKey: asset.SourceKey, Role: asset.Rule.Role, Lifeform: "human", Gender: "female", Description: "眼前穿深色教师长袍的人类女性教授。"}},
		Exchanges: []interactive.TurnCharacterExchangeInput{{CharacterID: "sinistra", Speaker: "character", Channel: "scene", AddressedTo: "protagonist", Text: "晚上好。"}, {CharacterID: "sinistra", Speaker: "character", Channel: "scene", AddressedTo: "protagonist", Text: "我是" + asset.Rule.KnownAs + "，" + asset.Rule.Role + "。"}},
		Knowledge: []interactive.TurnCharacterKnowledgeInput{{CharacterID: "protagonist", Statement: "亲眼看清她的人类女性形态，听她介绍姓名。", Nature: "experienced", Channel: "scene", Visual: &character.VisualObservationDraft{SubjectID: "sinistra", Form: "人类女性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}},
	}}})
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
	application, err := runtimeapp.New(t.Context(), &config.Config{OpenAIModel: "test", NovaDir: root, Workspace: root, ResumeLastWorkspace: false, OpenAIBaseURL: "http://127.0.0.1:1/v1", MediaDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	server := NewServer(application, "0")
	performRequest := ut.PerformRequest
	if executable := os.Getenv("TA_STORY_MEDIA_BINARY"); executable != "" {
		performRequest = startStoryMediaBinary(t, executable, root)
	}
	q := url.Values{"branch": {"main"}, "head": {interactive.StoryMediaHead(ctx.Snapshot)}, "revision": {fmt.Sprint(ctx.Snapshot.ContextRevision)}, "actor": {"sinistra"}}
	target := "/api/interactive/stories/" + story.ID + "/media/" + asset.ID + "?" + q.Encode()
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		result := performRequest(server.engine.Engine, method, target, nil)
		if result.Code != 200 {
			t.Fatalf("%s positive: %d %s", method, result.Code, result.Body.String())
		}
		if got := result.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
			t.Fatalf("private image cache: %q", got)
		}
		if method == http.MethodGet {
			got := sha256.Sum256(result.Body.Bytes())
			if hex.EncodeToString(got[:]) != asset.SHA256 {
				t.Fatal("real HTTP bytes differ from reviewed SHA")
			}
		}
	}
	ranged := performRequest(server.engine.Engine, http.MethodGet, target, nil, ut.Header{Key: "Range", Value: "bytes=0-7"})
	if ranged.Code != 206 || ranged.Body.Len() != 8 {
		t.Fatalf("Range=%d bytes=%d", ranged.Code, ranged.Body.Len())
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, query := range []string{"", strings.Replace(q.Encode(), "branch=main", "branch=other", 1), strings.Replace(q.Encode(), "head="+url.QueryEscape(interactive.StoryMediaHead(ctx.Snapshot)), "head=stale", 1)} {
			bad := "/api/interactive/stories/" + story.ID + "/media/" + asset.ID + "?" + query
			result := performRequest(server.engine.Engine, method, bad, nil, ut.Header{Key: "Range", Value: "bytes=0-7"})
			if result.Code != 404 || result.Body.Len() != 0 {
				t.Fatalf("unsafe URL %s: %d", bad, result.Code)
			}
		}
	}
	branchTarget := func(context interactive.StoryContext) string {
		values := url.Values{"branch": {context.Snapshot.BranchID}, "head": {interactive.StoryMediaHead(context.Snapshot)}, "revision": {fmt.Sprint(context.Snapshot.ContextRevision)}, "actor": {"sinistra"}}
		return "/api/interactive/stories/" + story.ID + "/media/" + asset.ID + "?" + values.Encode()
	}
	for _, test := range []struct {
		context interactive.StoryContext
		want    int
	}{{beforeContext, 404}, {afterContext, 200}} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			result := performRequest(server.engine.Engine, method, branchTarget(test.context), nil)
			if result.Code != test.want {
				t.Fatalf("fork %s %s=%d want%d", test.context.Snapshot.BranchID, method, result.Code, test.want)
			}
		}
	}
	// Old public routes must not serve these bytes even when a filename is known.
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		result := performRequest(server.engine.Engine, method, "/api/media/"+asset.File, nil, ut.Header{Key: "Range", Value: "bytes=0-7"})
		if result.Code != 404 {
			t.Fatalf("global bypass=%d", result.Code)
		}
	}
	window, err := application.CharacterWindow(runtimeapp.CharacterWindowRequest{StoryID: story.ID, BranchID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range window.Characters {
		if p.ID == "sinistra" {
			found = true
			if p.Avatar != "" || p.AvatarDisplay != target || p.ImageCaption == "" {
				t.Fatalf("explicit-open display mismatch: %+v", p)
			}
		}
	}
	if !found {
		t.Fatal("missing picture consumer")
	}
	// Runtime revocation must invalidate both byte reads and metadata in this
	// already-running App. Only mutate this test's own synthetic directory,
	// never an opt-in external real-source installation.
	if payload != nil {
		manifestPath := filepath.Join(dir, "story-assets.json")
		original, readErr := os.ReadFile(manifestPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		t.Cleanup(func() { _ = os.WriteFile(manifestPath, original, 0600) })
		for _, change := range []string{"review revoked", "rule removed", "pack hash revoked", "malformed", "deleted"} {
			t.Run("runtime manifest/"+change, func(t *testing.T) {
				var data map[string]any
				if err := json.Unmarshal(original, &data); err != nil {
					t.Fatal(err)
				}
				entry := data["assets"].([]any)[0].(map[string]any)
				switch change {
				case "review revoked":
					entry["reviewed"] = false
				case "rule removed":
					delete(entry, "rule")
				case "pack hash revoked":
					data["pack_hashes"] = map[string]any{}
				}
				modified, err := json.Marshal(data)
				if err != nil {
					t.Fatal(err)
				}
				if change == "malformed" {
					modified = []byte("broken JSON")
				}
				if change == "deleted" {
					err = os.Remove(manifestPath)
				} else {
					err = os.WriteFile(manifestPath, modified, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := os.WriteFile(manifestPath, original, 0600); err != nil {
						t.Fatal(err)
					}
				}()
				for _, method := range []string{http.MethodGet, http.MethodHead} {
					for _, rangeValue := range []string{"", "bytes=0-7"} {
						var headers []ut.Header
						if rangeValue != "" {
							headers = append(headers, ut.Header{Key: "Range", Value: rangeValue})
						}
						response := performRequest(server.engine.Engine, method, target, nil, headers...)
						if response.Code != 404 {
							t.Fatalf("revoked %s ranged=%q: %d", method, rangeValue, response.Code)
						}
					}
				}
				view, err := application.CharacterWindow(runtimeapp.CharacterWindowRequest{StoryID: story.ID, BranchID: "main"})
				if err != nil {
					t.Fatal(err)
				}
				for _, person := range view.Characters {
					if person.ID == "sinistra" && (person.Avatar != "" || person.AvatarDisplay != "" || person.AvatarLorePath != "" || person.ImageCaption != "") {
						t.Fatalf("revoked metadata still exposes an image or fallback: %+v", person)
					}
				}
			})
			if restored := performRequest(server.engine.Engine, http.MethodGet, target, nil); restored.Code != 200 {
				t.Fatalf("restored identical manifest no longer serves authorized bytes: %d", restored.Code)
			}
		}
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
		if p.ID == "sinistra" && p.AvatarDisplay != "" {
			t.Fatal("formal retraction still displays picture")
		}
	}
	if r := performRequest(server.engine.Engine, http.MethodHead, target, nil, ut.Header{Key: "Range", Value: "bytes=0-7"}); r.Code != 404 {
		t.Fatal("formal retraction leaked old HEAD/Range")
	}
	if err := reopened.RewindToTurnParent(story.ID, interactive.RewindTurnRequest{BranchID: "main", TurnID: turn.ID}); err != nil {
		t.Fatal(err)
	}
	stale := performRequest(server.engine.Engine, http.MethodGet, target, nil, ut.Header{Key: "Range", Value: "bytes=0-7"})
	if stale.Code != 404 {
		t.Fatalf("rewound source leaked=%d", stale.Code)
	}
	inherited := performRequest(server.engine.Engine, http.MethodGet, branchTarget(afterContext), nil)
	if inherited.Code != 200 {
		t.Fatalf("main rewind erased valid fork inheritance: %d", inherited.Code)
	}
}
