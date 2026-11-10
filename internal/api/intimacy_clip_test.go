package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"ta/config"
	runtimeapp "ta/internal/app"
	"ta/internal/character"
	"ta/internal/interactive"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestIntimacyClipRequiresCurrentAuthorizedActivity(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "media")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	payload := clipPayload()
	file := "private-test.mp4"
	if err := os.WriteFile(filepath.Join(dir, file), payload, 0600); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"schema":"ta.media/v2","entries":[{"world":"clip-test","source_key":"adult-person","role":"intimate","playback_approval":"local_owner","file":%q,"sha256":"%x","bytes":%d}]}`, file, sha256.Sum256(payload), len(payload))
	if err := os.WriteFile(filepath.Join(dir, "media.json"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(root)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "Media access fixture", PackBinding: &interactive.StoryPackBinding{PackID: "clip-test", ContentHash: "fixture", Scope: "clip-test"},
		PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: "adult-person", Name: "测试人物", Age: 25}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	group := character.Group{ID: "room", Type: character.GroupOther, Name: "Fixture", Channel: character.ChannelScene, Version: 1, Members: []character.GroupMember{{CharacterID: "actor", JoinedVersion: 1}}}
	activity := character.IntimacyActivity{ID: "activity", GroupID: group.ID, Phase: character.IntimacyActive, Version: 1, Participants: []character.IntimacyParticipant{{CharacterID: "actor", Status: character.IntimacyAccepted, Adult: true}}}
	if _, err := store.AppendCharacterFacts(story.ID, interactive.AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第一日", Profiles: []character.Profile{{ID: "actor", SourceKey: "adult-person", Name: "测试人物"}},
		Groups: []character.Group{group}, IntimacyActivities: []character.IntimacyActivity{activity},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	application, err := runtimeapp.New(t.Context(), &config.Config{NovaDir: root, Workspace: root, MediaDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	server := NewServer(application, "0")
	request := func(method, target string, headers ...ut.Header) *ut.ResponseRecorder {
		return ut.PerformRequest(server.engine.Engine, method, target, nil, headers...)
	}
	prefix := "/api/interactive/stories/" + story.ID + "/intimacy"
	window := request(http.MethodGet, prefix+"?activity_id=activity&branch=main")
	var data struct {
		RosterClips map[string][]runtimeapp.MediaClip `json:"roster_clips"`
	}
	if window.Code != 200 || json.Unmarshal(window.Body.Bytes(), &data) != nil || len(data.RosterClips["actor"]) != 1 {
		t.Fatalf("activity did not project its clip: %d %s", window.Code, window.Body.String())
	}
	clip := data.RosterClips["actor"][0].URL
	span := ut.Header{Key: "Range", Value: "bytes=12-24"}
	response := request(http.MethodGet, clip, span, ut.Header{Key: "Accept-Encoding", Value: "gzip"})
	if response.Code != 206 || !bytes.Equal(response.Body.Bytes(), payload[12:25]) || response.Header().Get("Content-Encoding") != "" || !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("private Range response: status=%d bytes=%d", response.Code, response.Body.Len())
	}
	if head := request(http.MethodHead, clip); head.Code != 200 || head.Body.Len() != 0 {
		t.Fatalf("private HEAD: %d", head.Code)
	}
	for _, path := range []string{"/api/media/" + file, strings.Replace(clip, "actor=actor", "actor=other", 1), strings.Replace(clip, "branch=main", "branch=missing", 1), strings.Replace(clip, "activity_id=activity", "activity_id=other", 1)} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			if denied := request(method, path, span); denied.Code != 404 {
				t.Fatalf("unscoped clip was served: %s %s status=%d", method, path, denied.Code)
			}
		}
	}
	if _, err := application.EndIntimacy(t.Context(), runtimeapp.IntimacyEndRequest{StoryID: story.ID, BranchID: "main", ActivityID: activity.ID}); err != nil {
		t.Fatal(err)
	}
	if ended := request(http.MethodGet, clip, span); ended.Code != 404 {
		t.Fatalf("old URL still served after the activity ended: %d", ended.Code)
	}
}
