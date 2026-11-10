package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"ta/config"
	runtimeapp "ta/internal/app"
	"ta/internal/character"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/media"
)

func managedPicturePNG(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// All files and facts are local deterministic fixtures. No shared installation,
// opt-in real image directory, or live model is used by these mutation tests.
func managedPictureWriteFixture(t *testing.T, visible bool) (*Server, string, []byte) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "media")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	payload := managedPicturePNG(t, color.RGBA{R: 210, A: 255})
	sum := sha256.Sum256(payload)
	sha := hex.EncodeToString(sum[:])
	const world, packHash = "test-world", "test-content-hash"
	const sourceKey, name, role = "hp-npc-sinistra", "奥罗拉·辛尼斯塔", "天文学教授"
	asset := media.StoryAsset{ID: "story-art", World: world, SourceKey: sourceKey, File: sha + ".png", SHA256: sha, Bytes: int64(len(payload)), Reviewed: true, Rule: &media.StoryRule{Kind: "observed_named_portrait", Year: 1991, Form: "人类女性", Lifeform: "human", Gender: "female", KnownAs: name, CanonicalName: name, Role: role, AdultIdentity: true}}
	manifest := media.StoryManifest{Schema: "ta.story-media/v1", PackHashes: map[string][]string{world: {packHash}}, PackWorlds: map[string]string{world: world}, Assets: []media.StoryAsset{asset}}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "story-assets.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, asset.File), payload, 0600); err != nil {
		t.Fatal(err)
	}
	// Seed both ignored overrides through the real library before opening App.
	// Clear must not delete these merely because the player cannot see them.
	library := media.NewLibrary(dir)
	ignored := managedPicturePNG(t, color.RGBA{G: 120, A: 255})
	for _, purpose := range []string{media.PurposeAvatar, media.PurposePortrait} {
		if _, err := library.Import(media.ImportRequest{World: world, SourceKey: sourceKey, Subject: media.SubjectCharacter, Purpose: purpose, Filename: "ignored.png", Data: ignored}); err != nil {
			t.Fatal(err)
		}
	}
	store := interactive.NewStore(root)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "受管画像写入验收", PackBinding: &interactive.StoryPackBinding{PackID: world, ContentHash: packHash, Scope: "media-test"}, PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: sourceKey, Name: name, Role: role, Gender: "female", Lifeform: "human"}}}, InitialStateOps: []interactivestate.Op{
		{Op: "set", Path: "actors.story.id", Value: "story"}, {Op: "set", Path: "actors.story.state.当前时间", Value: "1991年9月1日"}, {Op: "set", Path: "actors.story.state.当前详细地点", Value: "天文塔"}, {Op: "set", Path: "actors.story.state.在场角色", Value: []any{"protagonist", "sinistra", "ordinary"}},
		{Op: "set", Path: "actors.sinistra.id", Value: "sinistra"}, {Op: "set", Path: "actors.sinistra.name", Value: name}, {Op: "set", Path: "actors.sinistra.state.外貌与装扮", Value: "人类女性，穿深色教师长袍"},
		{Op: "set", Path: "actors.ordinary.id", Value: "ordinary"}, {Op: "set", Path: "actors.ordinary.name", Value: "普通访客"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	facts := &interactive.TurnCharacterFactsInput{Profiles: []interactive.TurnCharacterProfileInput{
		{CharacterID: "sinistra", Name: name, KnownAs: name, SourceKey: sourceKey, Role: role, Lifeform: "human", Gender: "female", Description: "眼前穿深色教师长袍的人类女性教授。"},
		{CharacterID: "ordinary", Name: "普通访客", KnownAs: "普通访客", Role: "访客", Lifeform: "human", Gender: "male"},
	}}
	if visible {
		facts.Exchanges = []interactive.TurnCharacterExchangeInput{{CharacterID: "sinistra", Speaker: "character", Channel: "scene", AddressedTo: "protagonist", Text: "晚上好。"}, {CharacterID: "sinistra", Speaker: "character", Channel: "scene", AddressedTo: "protagonist", Text: "我是" + name + "，" + role + "。"}}
		facts.Knowledge = []interactive.TurnCharacterKnowledgeInput{{CharacterID: "protagonist", Statement: "亲眼看清她的人类女性形态，听她介绍姓名。", Nature: "experienced", Channel: "scene", Visual: &character.VisualObservationDraft{SubjectID: "sinistra", Form: "人类女性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}}
	}
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "观察眼前的人", Narrative: "你在天文塔与眼前的人停留。", TurnResult: &interactive.TurnResult{Choices: []string{"交谈", "观察", "询问", "等待", "离开"}, CharacterFacts: facts}}); err != nil {
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
	picture, err := application.CharacterPicture(story.ID, "main", "sinistra")
	if err != nil || picture.Source != "story" || (picture.URL != "") != visible || picture.Overridden {
		t.Fatalf("fixture visibility mismatch: visible=%v picture=%+v err=%v", visible, picture, err)
	}
	return NewServer(application, "0"), story.ID, managedPicturePNG(t, color.RGBA{B: 230, A: 255})
}

func pictureDirectoryBytes(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			out[rel+"/"] = nil
			return nil
		}
		out[rel], err = os.ReadFile(path)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertPictureDirectoryUnchanged(t *testing.T, dir string, before map[string][]byte) {
	t.Helper()
	if after := pictureDirectoryBytes(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatal("rejected picture operation changed media files or manifest bytes")
	}
}

func TestStoryManagedPictureAppWritesRefusedWithoutMutation(t *testing.T) {
	for _, visible := range []bool{false, true} {
		t.Run(fmt.Sprint("visible=", visible), func(t *testing.T) {
			server, storyID, upload := managedPictureWriteFixture(t, visible)
			dir := server.app.Media().Dir()
			before := pictureDirectoryBytes(t, dir)
			for _, purpose := range []string{media.PurposeAvatar, media.PurposePortrait} {
				request := runtimeapp.CharacterPictureImport{StoryID: storyID, BranchID: "main", CharacterID: "sinistra", Purpose: purpose, Filename: "new.png", Data: upload}
				for _, operation := range []func(runtimeapp.CharacterPictureImport) (runtimeapp.CharacterPictureImportResult, error){server.app.CharacterPictureImport, server.app.CharacterPictureClear} {
					result, err := operation(request)
					if err == nil || err.Error() != "this portrait is managed by story rules and cannot be replaced or reset" {
						t.Fatalf("managed operation did not return stable refusal: result=%+v err=%v", result, err)
					}
					if !reflect.DeepEqual(result, runtimeapp.CharacterPictureImportResult{}) {
						t.Fatalf("refused write returned image data: %+v", result)
					}
					assertPictureDirectoryUnchanged(t, dir, before)
				}
				// Ordinary characters retain real import and restore behavior.
				request.CharacterID = "ordinary"
				imported, err := server.app.CharacterPictureImport(request)
				if err != nil || imported.URL == "" || imported.Picture.Source != runtimeapp.PictureSourceExternal || !imported.Picture.Overridden {
					t.Fatalf("ordinary app import failed: %+v %v", imported, err)
				}
				current, err := server.app.CharacterPicture(storyID, "main", "ordinary")
				if err != nil || current.URL != imported.URL {
					t.Fatalf("ordinary app import was not displayed: %+v %v", current, err)
				}
				cleared, err := server.app.CharacterPictureClear(request)
				if err != nil || cleared.Picture.Overridden || cleared.Picture.URL != "" {
					t.Fatalf("ordinary app restore failed: %+v %v", cleared, err)
				}
				// Import can leave a recoverable hash file by design. Freeze the
				// new legitimate directory state before the next refused operation.
				before = pictureDirectoryBytes(t, dir)
			}
		})
	}
}

func postPictureMultipart(t *testing.T, server *Server, path, purpose, locale string, upload []byte) *ut.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range map[string]string{"purpose": purpose, "source_key": "ordinary-upload", "world": "test-world"} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if upload != nil {
		part, err := writer.CreateFormFile("file", "new.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(upload); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return ut.PerformRequest(server.engine.Engine, http.MethodPost, path, &ut.Body{Body: bytes.NewReader(buf.Bytes()), Len: buf.Len()}, ut.Header{Key: "Content-Type", Value: writer.FormDataContentType()}, ut.Header{Key: "X-TA-Locale", Value: locale})
}

func TestStoryManagedPictureHTTPWritesRefusedWithoutMutation(t *testing.T) {
	for _, visible := range []bool{false, true} {
		t.Run(fmt.Sprint("visible=", visible), func(t *testing.T) {
			server, storyID, upload := managedPictureWriteFixture(t, visible)
			dir := server.app.Media().Dir()
			before := pictureDirectoryBytes(t, dir)
			path := "/api/interactive/stories/" + storyID + "/characters/sinistra/picture"
			for _, purpose := range []string{media.PurposeAvatar, media.PurposePortrait} {
				for _, locale := range []string{"zh-CN", "en-US"} {
					for _, operation := range []string{"", "/clear"} {
						data := upload
						if operation == "/clear" {
							data = nil
						}
						result := postPictureMultipart(t, server, path+operation+"?branch=main", purpose, locale, data)
						if result.Code != http.StatusConflict {
							t.Fatalf("managed HTTP %s returned %d: %s", operation, result.Code, result.Body.String())
						}
						assertAgentRuntimeErrorCode(t, result.Body.Bytes(), "api.media.storyManaged")
						message := "该画像由故事规则管理，不能上传替换或恢复默认。"
						if locale == "en-US" {
							message = "This portrait is managed by story rules and cannot be replaced or reset."
						}
						if !bytes.Contains(result.Body.Bytes(), []byte(message)) {
							t.Fatalf("missing localized refusal: %s", result.Body.String())
						}
						assertPictureDirectoryUnchanged(t, dir, before)
					}
				}
			}
			ordinaryPath := "/api/interactive/stories/" + storyID + "/characters/ordinary/picture"
			for _, purpose := range []string{media.PurposeAvatar, media.PurposePortrait} {
				result := postPictureMultipart(t, server, ordinaryPath+"?branch=main", purpose, "en-US", upload)
				if result.Code != http.StatusOK {
					t.Fatalf("ordinary HTTP import=%d %s", result.Code, result.Body.String())
				}
				var imported runtimeapp.CharacterPictureImportResult
				decodeResponse(t, result.Body.Bytes(), &imported)
				if imported.URL == "" || imported.Picture.Source != runtimeapp.PictureSourceExternal || !imported.Picture.Overridden {
					t.Fatalf("ordinary HTTP import not reflected: %+v", imported)
				}
				image := ut.PerformRequest(server.engine.Engine, http.MethodGet, imported.URL, nil)
				if image.Code != http.StatusOK || !bytes.Equal(image.Body.Bytes(), upload) {
					t.Fatalf("ordinary imported image not served: %d", image.Code)
				}
				result = postPictureMultipart(t, server, ordinaryPath+"/clear?branch=main", purpose, "en-US", nil)
				if result.Code != http.StatusOK {
					t.Fatalf("ordinary HTTP restore=%d %s", result.Code, result.Body.String())
				}
				var cleared runtimeapp.CharacterPictureImportResult
				decodeResponse(t, result.Body.Bytes(), &cleared)
				if cleared.Picture.Overridden || cleared.Picture.URL != "" {
					t.Fatalf("ordinary HTTP restore not reflected: %+v", cleared)
				}
			}
		})
	}
}

func TestStoryRestrictedImportAppAndHTTPDoNotWrite(t *testing.T) {
	for _, change := range []string{"unchanged", "asset removed", "malformed", "deleted"} {
		t.Run(change, func(t *testing.T) {
			server, storyID, ordinaryUpload := managedPictureWriteFixture(t, false)
			dir := server.app.Media().Dir()
			path := filepath.Join(dir, "story-assets.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var manifest media.StoryManifest
			if err := json.Unmarshal(raw, &manifest); err != nil {
				t.Fatal(err)
			}
			if len(manifest.Assets) != 1 {
				t.Fatal("fixture must contain one restricted source")
			}
			privateUpload, err := os.ReadFile(filepath.Join(dir, manifest.Assets[0].File))
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "asset removed":
				manifest.Assets = nil
				raw, err = json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
			case "malformed":
				raw = []byte("broken manifest")
			}
			if change == "deleted" {
				err = os.Remove(path)
			} else if change != "unchanged" {
				err = os.WriteFile(path, raw, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			before := pictureDirectoryBytes(t, dir)
			payloads := [][]byte{privateUpload}
			if change != "unchanged" {
				// An invalidated installed generation must reject even a new hash.
				payloads = append(payloads, ordinaryUpload)
			}
			for _, payload := range payloads {
				result, err := server.app.CharacterPictureImport(runtimeapp.CharacterPictureImport{StoryID: storyID, BranchID: "main", CharacterID: "ordinary", Purpose: media.PurposePortrait, Filename: "new.png", Data: payload})
				if err == nil || err.Error() != "story media restrictions do not allow this file to be imported" || !reflect.DeepEqual(result, runtimeapp.CharacterPictureImportResult{}) {
					t.Fatalf("restricted app import did not refuse: %+v %v", result, err)
				}
				assertPictureDirectoryUnchanged(t, dir, before)
				for _, target := range []string{"/api/interactive/stories/" + storyID + "/characters/ordinary/picture?branch=main", "/api/media-assets/import"} {
					for _, locale := range []string{"zh-CN", "en-US"} {
						response := postPictureMultipart(t, server, target, media.PurposePortrait, locale, payload)
						if response.Code != http.StatusConflict {
							t.Fatalf("restricted HTTP import=%d %s", response.Code, response.Body.String())
						}
						assertAgentRuntimeErrorCode(t, response.Body.Bytes(), "api.media.storyRestricted")
						message := "故事素材限制不允许导入此文件，请选择其他图片或检查素材包。"
						if locale == "en-US" {
							message = "Story media restrictions prevent this import. Choose another image or check the media pack."
						}
						if !bytes.Contains(response.Body.Bytes(), []byte(message)) {
							t.Fatalf("missing localized restricted import refusal: %s", response.Body.String())
						}
						assertPictureDirectoryUnchanged(t, dir, before)
					}
				}
			}
			if change == "unchanged" {
				response := postPictureMultipart(t, server, "/api/media-assets/import", media.PurposePortrait, "en-US", ordinaryUpload)
				if response.Code != http.StatusOK {
					t.Fatalf("ordinary global import=%d %s", response.Code, response.Body.String())
				}
				var imported media.Imported
				decodeResponse(t, response.Body.Bytes(), &imported)
				image := ut.PerformRequest(server.engine.Engine, http.MethodGet, imported.URL, nil)
				if image.Code != http.StatusOK || !bytes.Equal(image.Body.Bytes(), ordinaryUpload) {
					t.Fatalf("ordinary global import did not display: %d", image.Code)
				}
			}
		})
	}
}
