package app

import (
 "encoding/json"
 "fmt"
 "os"
 "reflect"
 "testing"
 "ta/internal/character"
 "ta/internal/interactive"
 "ta/internal/media"
)

func portraitExtensionFixture(t *testing.T, asset media.StoryAsset) (media.StoryAsset, interactive.StoryContext, character.State) {
	t.Helper()
	p := character.Profile{ID: "portrait-subject", StoryID: "st", Name: asset.Rule.CanonicalName, KnownAs: asset.Rule.KnownAs, SourceKey: asset.SourceKey, Role: asset.Rule.Role, Lifeform: asset.Rule.Lifeform, Gender: asset.Rule.Gender, Revision: 1}
	appearance := asset.Rule.Form + "，穿日常衣服"
	scene := character.Scene{NodeID: "turn", WorldTime: "1991年9月1日", Location: "已提交的公开街道接触地点", Present: []string{"protagonist", "portrait-subject"}}
	n := 0
	facts := character.TurnCharacterFacts{Exchanges: []character.ExchangeDraft{{CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "晚上好。"}, {CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "我是" + p.KnownAs + "，" + p.Role + "。"}}, Knowledge: []character.KnowledgeDraft{{CharacterID: "protagonist", Statement: "亲眼看见并认识此人。", Nature: character.NatureExperienced, Channel: character.ChannelScene, Visual: &character.VisualObservationDraft{SubjectID: p.ID, Form: asset.Rule.Form, ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}}}
	projected, err := character.Project(character.ProjectionContext{StoryID: "st", BranchID: "main", TurnID: "turn", CommitID: "commit", WorldTime: scene.WorldTime, Scene: scene, KnownCharacters: []string{p.ID}, VisualContext: &character.VisualObservationContext{ObserverID: "protagonist", Subjects: map[string]character.VisualSubject{p.ID: {Profile: p, Appearance: appearance}}}, NextID: func(prefix string) string { n++; return fmt.Sprintf("%s%d", prefix, n) }}, facts)
	if err != nil {
		t.Fatal(err)
	}
	state := character.State{Profiles: []character.Profile{p}, Exchanges: projected.Exchanges, Knowledge: projected.Knowledge}
	ctx := interactive.StoryContext{Meta: interactive.StoryMeta{PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: p.SourceKey, Name: p.Name}}}}, Snapshot: interactive.Snapshot{StoryID: "st", BranchID: "main", CurrentTurn: &interactive.TurnEvent{ID: "turn"}, State: map[string]any{"actors": map[string]any{"story": map[string]any{"state": map[string]any{"当前时间": scene.WorldTime, "当前详细地点": scene.Location, "在场角色": []any{"protagonist", p.ID}}}, p.ID: map[string]any{"state": map[string]any{"外貌与装扮": appearance}}}}}}

	return asset, ctx, state
}
// Tests use formal deterministic fixtures, not model or real-story acceptance.
func TestPortraitExtensionMappings(t *testing.T) {
 path := os.Getenv("TA_PORTRAIT_EXTENSION_MANIFEST")
 if path == "" { t.Skip("set TA_PORTRAIT_EXTENSION_MANIFEST to the merged manifest for mapping integration tests") }
 raw, err := os.ReadFile(path); if err != nil { t.Fatal(err) }
 var manifest struct { Assets []media.StoryAsset `json:"assets"` }
 if err := json.Unmarshal(raw, &manifest); err != nil { t.Fatal(err) }
 expected := map[string]string{
  "p11o-art-hp-person-lucius-v1": "马尔福家长",
  "p11o-art-hp-person-rosmerta-v1": "三把扫帚经营者",
  "p11q-art-hp-person-vernon-v1": "哈利的姨父",
 }
 for id, role := range expected {
  t.Run(id, func(t *testing.T) {
   var found *media.StoryAsset
   for i := range manifest.Assets { if manifest.Assets[i].ID == id { found = &manifest.Assets[i]; break } }
   if found == nil || found.Rule == nil || found.Rule.Role != role || found.Rule.Year != 1991 || !found.Reviewed { t.Fatal("missing or altered extension rule") }
   asset, ctx, state := portraitExtensionFixture(t, *found)
   before, _ := json.Marshal(state); snap, _ := json.Marshal(ctx)
   if !storyAssetAllowed(asset, ctx, state, "portrait-subject") { t.Fatal("formal public encounter denied") }
   after, _ := json.Marshal(state); snapAfter, _ := json.Marshal(ctx)
   if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(snap, snapAfter) { t.Fatal("picture gate mutated knowledge, role, permission or scene") }
   cases := map[string]func(*interactive.StoryContext, *character.State){
    "no committed encounter": func(c *interactive.StoryContext,s *character.State){s.Knowledge=nil},
    "no recognition": func(c *interactive.StoryContext,s *character.State){s.Profiles[0].KnownAs=""},
    "wrong role": func(c *interactive.StoryContext,s *character.State){s.Profiles[0].Role="不匹配"},
    "wrong source": func(c *interactive.StoryContext,s *character.State){s.Profiles[0].SourceKey="hp-npc-other"},
    "wrong year": func(c *interactive.StoryContext,s *character.State){c.Snapshot.State["actors"].(map[string]any)["story"].(map[string]any)["state"].(map[string]any)["当前时间"]="1992年9月1日"},
    "not present": func(c *interactive.StoryContext,s *character.State){c.Snapshot.State["actors"].(map[string]any)["story"].(map[string]any)["state"].(map[string]any)["在场角色"]=[]any{"protagonist"}},
    "retracted": func(c *interactive.StoryContext,s *character.State){s.Knowledge[0].RetractedBy="withdrawn"},
    "unknown current appearance": func(c *interactive.StoryContext,s *character.State){c.Snapshot.State["actors"].(map[string]any)["portrait-subject"].(map[string]any)["state"].(map[string]any)["外貌与装扮"]=""},
   }
   for name, mutate := range cases { t.Run(name,func(t *testing.T) {
    a,c,s:=portraitExtensionFixture(t,*found); mutate(&c,&s)
    if storyAssetAllowed(a,c,s,"portrait-subject") { t.Fatal("missing source accepted") }
   }) }
  })
 }
 blocked := map[string]bool{
 "p11o-art-hp-person-bill-v1":true,"p11o-art-hp-person-charlie-v1":true,
 "p11o-art-hp-person-narcissa-v1":true,"p11q-art-hp-person-petunia-v1":true,
 "p11q-art-hp-person-augusta-v1":true,
 "p11q-art-hp-person-borgin-v1":true,"p11q-art-hp-person-fudge-v1":true,
 "p11q-art-hp-person-crouch-v1":true,"p11q-art-hp-person-bagman-v1":true,
 "p11q-art-hp-person-umbridge-v1":true,"p11q-art-hp-person-kingsley-v1":true,
 "p11q-art-hp-person-scrimgeour-v1":true,"p11q-art-hp-person-lockhart-v1":true,
 }
 for _, asset := range manifest.Assets { if blocked[asset.ID] && asset.Rule != nil { t.Fatalf("unrepresented condition silently enabled: %s",asset.ID) } }
}
