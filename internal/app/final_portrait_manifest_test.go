package app

import (
	"encoding/json"
	"fmt"
	"os"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
	"testing"
)

// This exercises the actual packaged manifest against exact production gate
// code with deterministic source records. It is not a real model encounter.
func TestFinalPortraitManifestRules(t *testing.T) {
	path := os.Getenv("TA_FINAL_PORTRAIT_MANIFEST")
	if path == "" {
		t.Skip("set TA_FINAL_PORTRAIT_MANIFEST to packaged story-assets.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Assets []media.StoryAsset `json:"assets"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	count := 0
	seen := map[string]bool{}
	for _, asset := range manifest.Assets {
		if asset.Rule == nil {
			continue
		}
		if !asset.Reviewed || seen[asset.ID] {
			t.Fatalf("rule is not reviewed or asset ID is duplicated: %s", asset.ID)
		}
		seen[asset.ID] = true
		count++
		t.Run(asset.ID, func(t *testing.T) {
			r := asset.Rule
			when := fmt.Sprintf("%d年9月1日", r.Year)
			if r.TimeSemantics == "current_observation" {
				when = "本局第1日清晨"
				if asset.World == "lotm" {
					when = "1349年9月1日"
				}
			}
			spoken := "我叫" + r.KnownAs + "。"
			if r.RoleSemantics != "background_classification" {
				title := r.Role
				if r.RoleRecognition != "" {
					title = r.RoleRecognition
				}
				spoken = "我是" + r.KnownAs + "，" + title + "。"
			}
			spec := approvedMappingCase{assetID: asset.ID, source: asset.SourceKey, name: r.KnownAs, role: r.Role, gender: r.Gender, form: r.Form, timeSemantics: r.TimeSemantics, roleSemantics: r.RoleSemantics, roleRecognition: r.RoleRecognition, recognition: spoken, worldTime: when, year: r.Year, adult: r.AdultIdentity}
			location := "已提交的实际会面地点"
			switch r.Kind {
			case "observed_named_portrait":
			case "observed_named_portrait_at_location":
				if r.Location == "" {
					t.Fatal("location-bound manifest rule has no authored location")
				}
				location = r.Location
			default:
				t.Fatalf("unsupported manifest kind: %q", r.Kind)
			}
			observationAt := func(t *testing.T, place string) (interactive.StoryContext, character.State) {
				t.Helper()
				fixture := spec
				fixture.location = place
				_, ctx, state := approvedMappingFixture(t, fixture)
				state.Profiles[0].Name = r.CanonicalName
				ctx.Meta.PackCatalog.Characters[0].Name = r.CanonicalName
				if interactive.StoryScene(ctx.Snapshot).Location != place || len(state.Knowledge) != 1 || state.Knowledge[0].Visual == nil || state.Knowledge[0].Visual.Location != place || state.Knowledge[0].FormalCommitID == "" || !state.Knowledge[0].Live() {
					t.Fatal("formal projection did not retain a complete same-location observation")
				}
				return ctx, state
			}
			ctx, state := observationAt(t, location)
			if r.Kind == "observed_named_portrait_at_location" {
				for _, tc := range []struct{ name, location string }{
					{"wrong_location", "隔离验收错误会面地点"},
					{"nearby", r.Location + "附近"},
					{"not_arrived", "尚未到达" + r.Location},
				} {
					t.Run(tc.name, func(t *testing.T) {
						badContext, completeState := observationAt(t, tc.location)
						if storyAssetAllowed(asset, badContext, completeState, "sinistra") {
							t.Fatal("complete matching observation bypassed the authored location")
						}
						// Removing only the test-local rule restriction proves the denial
						// was not caused by missing identity, provenance, or snapshot drift.
						control, legacyRule := asset, *r
						legacyRule.Kind, legacyRule.Location = "observed_named_portrait", ""
						control.Rule = &legacyRule
						if !storyAssetAllowed(control, badContext, completeState, "sinistra") {
							t.Fatal("negative location fixture also failed an unrelated gate")
						}
					})
				}
				t.Run("empty_authored_location", func(t *testing.T) {
					// The committed observation stays valid. Blank scene locations
					// cannot produce formal visuals and are tested at the API commit path.
					badAsset, emptyRule := asset, *r
					emptyRule.Location = ""
					badAsset.Rule = &emptyRule
					if storyAssetAllowed(badAsset, ctx, state, "sinistra") {
						t.Fatal("empty authored restriction accepted valid observed facts")
					}
				})
			}
			if !storyAssetAllowed(asset, ctx, state, "sinistra") {
				t.Fatal("actual manifest rule rejected valid deterministic observation")
			}
			state.Knowledge[0].RetractedBy = "withdrawn"
			if storyAssetAllowed(asset, ctx, state, "sinistra") {
				t.Fatal("retracted actual manifest observation allowed")
			}
			state.Knowledge[0].RetractedBy = ""
			state.Profiles[0].Gender = "unspecified"
			if storyAssetAllowed(asset, ctx, state, "sinistra") {
				t.Fatal("unknown current gender authorized by image metadata")
			}
			state.Profiles[0].Gender = r.Gender
			if r.RoleSemantics != "background_classification" {
				state.Exchanges[1].Text = "我叫" + r.KnownAs + "。"
				if storyAssetAllowed(asset, ctx, state, "sinistra") {
					t.Fatal("missing actual role source allowed")
				}
			}
		})
	}
	if count != 99 {
		t.Fatalf("expected 99 reviewed rules, got %d", count)
	}
}
