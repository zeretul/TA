package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ta/internal/agents/modelio"
	"ta/internal/storypack"
)

// 自制-AI创建世界包与武侠规则复用 §2 asks the two entry points to write into one
// shared draft. These tests cover the merge rule, which is the part that decides
// whether switching entry point or retrying a failed call loses the player's
// work.

func TestAWorldDraftKeepsWhatThePlayerAlreadyWrote(t *testing.T) {
	draft := storypack.WorldDraft{
		Name:   "寒山驿",
		Origin: "玩家自己写的开局。",
		World:  []storypack.DraftWorldEntry{{Name: "寒山驿", Content: "玩家写的正文。"}},
	}
	payload := worldDraftPayload{
		Name:   "雪岭驿",
		Origin: "模型写的开局。",
		Title:  "雪停之前",
	}
	merged := mergeWorldDraft(draft, payload, false, worldDraftSeed{})

	if merged.Name != "寒山驿" {
		t.Fatalf("the model renamed a world the player had already named: %q", merged.Name)
	}
	if merged.Origin != "玩家自己写的开局。" {
		t.Fatalf("the model overwrote the player's premise: %q", merged.Origin)
	}
	if merged.Title != "雪停之前" {
		t.Fatalf("a field the player left blank was not filled: %q", merged.Title)
	}
	if len(merged.World) != 1 || merged.World[0].Content != "玩家写的正文。" {
		t.Fatalf("the world book was replaced although the player had written one: %#v", merged.World)
	}
}

func TestARewriteReplacesTheProseButKeepsTheSeed(t *testing.T) {
	draft := storypack.WorldDraft{
		Name:   "寒山驿",
		Origin: "旧的开局。",
		World:  []storypack.DraftWorldEntry{{Name: "寒山驿", Content: "旧正文。"}},
	}
	payload := worldDraftPayload{
		Name:   "雪岭驿",
		Origin: "新的开局。",
		World:  []worldDraftEntryPayload{{Name: "寒山驿", Content: "新正文。"}},
	}
	merged := mergeWorldDraft(draft, payload, true, worldDraftSeed{Name: "寒山驿"})

	if merged.Origin != "新的开局。" {
		t.Fatalf("a rewrite did not replace the prose: %q", merged.Origin)
	}
	if len(merged.World) != 1 || merged.World[0].Content != "新正文。" {
		t.Fatalf("a rewrite did not replace the world book: %#v", merged.World)
	}
	// The player's own seed still wins over the model, even on a rewrite.
	if merged.Name != "寒山驿" {
		t.Fatalf("a rewrite replaced the name the player typed: %q", merged.Name)
	}
}

func TestAnUnsupportedStartingPositionIsDropped(t *testing.T) {
	payload := worldDraftPayload{}
	payload.Origins = []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Basis       string `json:"basis"`
		Technique   string `json:"technique"`
		Item        string `json:"item"`
	}{
		{Name: "散修", Basis: "wanderer"},
		{Name: "剑仙", Basis: "sword-immortal"},
		{Name: "门人", Basis: "disciple"},
		{Name: "重复散修", Basis: "wanderer"},
	}
	origins := mergeWorldOrigins(payload.Origins)
	if len(origins) != 2 {
		t.Fatalf("want the two supported positions, got %#v", origins)
	}
	if origins[0].Basis != "wanderer" || origins[1].Basis != "disciple" {
		t.Fatalf("an unsupported position was kept or a supported one dropped: %#v", origins)
	}
}

func TestTheUnaffiliatedOptionIsNotDuplicated(t *testing.T) {
	options := mergeWorldOptions([]string{"驿马帮", "无门无派", "", "  "})
	if len(options) != 1 || options[0].Name != "驿马帮" {
		t.Fatalf("the program's own option was duplicated or a blank was kept: %#v", options)
	}
}

func TestAFencedWorldDraftIsRead(t *testing.T) {
	payload, err := parseWorldDraftPayload("```json\n{\"name\":\"寒山驿\",\"origin\":\"雪夜到驿。\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if payload.Name != "寒山驿" || payload.Origin != "雪夜到驿。" {
		t.Fatalf("the fenced object was not read: %#v", payload)
	}
}

func TestAWorldDraftWithNoPremiseIsAnError(t *testing.T) {
	if _, err := parseWorldDraftPayload(`{"name":"寒山驿"}`); err == nil {
		t.Fatal("a draft that wrote no premise must be reported, not accepted as an empty world")
	}
	if _, err := parseWorldDraftPayload("这不是 JSON"); err == nil {
		t.Fatal("a non-object response must be reported")
	}
}

func TestWorldDraftTransportFailureUsesALocalizedKey(t *testing.T) {
	technical := fmt.Errorf(`generate world draft: Post "http://127.0.0.1:8317/v1/chat/completions": connection refused`)
	if got := worldDraftErrorValue(technical); got != "worldCreation.draftFailed" {
		t.Fatalf("the UI error exposes transport details instead of a localized key: %q", got)
	}
}

// TestTheWorldDraftSeedWinsOverTheModel keeps the player's own answers from
// being rewritten by a later call.
func TestTheWorldDraftSeedWinsOverTheModel(t *testing.T) {
	merged := mergeWorldDraft(
		storypack.WorldDraft{},
		worldDraftPayload{Name: "模型起的名字", Protagonist: "模型写的主角。"},
		false,
		worldDraftSeed{Name: "玩家起的名字", Protagonist: "玩家写的主角。"},
	)
	if merged.Name != "玩家起的名字" {
		t.Fatalf("the model overrode the player's name: %q", merged.Name)
	}
	if merged.Protagonist != "玩家写的主角。" {
		t.Fatalf("the model overrode the player's protagonist: %q", merged.Protagonist)
	}
}

// TestAMissingModelIsReportedInWordsAPlayerCanActOn keeps provider resolution
// detail out of the page. The page has one thing to say — go and set a model up
// — and the protocol name belongs in the log, not in a dialog.
func TestAMissingModelIsReportedInWordsAPlayerCanActOn(t *testing.T) {
	technical := fmt.Errorf(`resolve world creation model configuration: resolve model: provider "openai-compatible" protocol "openai-chat-completions" requires a base URL`)
	got := modelUnavailable(context.Background(), "world-creation", "zh-CN", technical)
	if !errors.Is(got, ErrNoDraftingModel) {
		t.Fatalf("a missing model was not reported as one: %v", got)
	}
	if pageMessage := worldDraftErrorValue(got); pageMessage != got.Error() {
		t.Fatalf("the actionable missing-model copy was not preserved: %q", pageMessage)
	}
	for _, leaked := range []string{"openai-compatible", "base URL", "resolve model", "protocol"} {
		if strings.Contains(got.Error(), leaked) {
			t.Fatalf("the refusal still carries %q: %s", leaked, got.Error())
		}
	}
}

// TestTheMissingModelRefusalIsWrittenInThePlayersLanguage keeps the wording in
// the game's own voice in both languages: the player is told there is no
// language model connected and where to connect one, not that something is
// missing that they have to "write with".
func TestTheMissingModelRefusalIsWrittenInThePlayersLanguage(t *testing.T) {
	technical := fmt.Errorf("resolve model: provider %q requires a base URL", "openai-compatible")
	zh := modelUnavailable(context.Background(), "world-creation", "zh-CN", technical).Error()
	en := modelUnavailable(context.Background(), "world-creation", "en-US", technical).Error()
	if !strings.Contains(zh, "设置") || !strings.Contains(zh, "API Key") {
		t.Fatalf("the Chinese refusal does not say where to fix it: %s", zh)
	}
	if !strings.Contains(en, "settings") || !strings.Contains(en, "API key") {
		t.Fatalf("the English refusal does not say where to fix it: %s", en)
	}
	if strings.Contains(zh, "能写作") {
		t.Fatalf("the refusal still uses the old wording: %s", zh)
	}
	if zh == en {
		t.Fatalf("both languages got the same text: %s", zh)
	}
}

// TestADraftTooLongForTheModelIsNotAMissingModel keeps the two refusals apart: an
// oversized draft has a model and only needs to be shorter, so telling the player
// to go and configure one would send them the wrong way.
func TestADraftTooLongForTheModelIsNotAMissingModel(t *testing.T) {
	tooLong := &modelio.ProviderInputLimitError{AgentKind: "interactive", Bytes: 900, MaxBytes: 100}
	got := modelUnavailable(context.Background(), "world-creation", "zh-CN", tooLong)
	if errors.Is(got, ErrNoDraftingModel) {
		t.Fatal("an oversized draft was reported as a missing model")
	}
	if !strings.Contains(got.Error(), "context limit") {
		t.Fatalf("the size refusal was not kept: %v", got)
	}
	if gotKey := worldDraftErrorValue(got); gotKey != "worldCreation.draftTooLong" {
		t.Fatalf("the page error should use localized input-limit copy, got %q", gotKey)
	}
}

func TestGeneratedWorldPreservesLoreRouting(t *testing.T) {
	payload, err := parseWorldDraftPayload(`{"origin":"访客抵达。","world":[{"name":"公开规则","type":"rule","content":"公开条件","load_mode":"resident"},{"name":"秘密","type":"rule","content":"尚未知晓","load_mode":"manual","keywords":["密室"]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	entries := mergeWorldEntries(payload.World)
	if len(entries) != 2 || entries[0].LoadMode != "resident" || entries[1].LoadMode != "manual" || len(entries[1].Keywords) != 1 {
		t.Fatalf("lost lore routing: %+v", entries)
	}
}
