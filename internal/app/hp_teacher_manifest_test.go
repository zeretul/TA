package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"ta/internal/media"
	"testing"
)

// This uses the three real packaged rules. Fixtures supply synthetic committed
// sources; passing it does not substitute for a real-model or HTTP playthrough.
func TestHPTeacherReviewedPublicTitlesActualManifest(t *testing.T) {
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
	for _, asset := range manifest.Assets {
		r := asset.Rule
		if r == nil || len(r.KnownAsAliases) == 0 {
			continue
		}
		count++
		if asset.World != "harry_potter" || len(r.KnownAsAliases) != 1 || r.Year != 1991 || r.RoleRecognition == "" {
			t.Fatalf("unexpected alias mapping: %s", asset.ID)
		}
		if strings.Contains(asset.Caption, r.CanonicalName) {
			t.Fatal("caption reveals canonical name")
		}
		for _, alias := range r.KnownAsAliases {
			t.Run(alias, func(t *testing.T) {
				spec := approvedMappingCase{assetID: asset.ID, source: asset.SourceKey, name: alias, role: r.Role, gender: r.Gender, form: r.Form, roleRecognition: r.RoleRecognition, recognition: "我是" + alias + "，" + r.RoleRecognition + "。", worldTime: fmt.Sprintf("%d年9月1日", r.Year), year: r.Year, adult: r.AdultIdentity}
				_, ctx, state := approvedMappingFixture(t, spec)
				state.Profiles[0].Name = r.CanonicalName
				ctx.Meta.PackCatalog.Characters[0].Name = r.CanonicalName
				if !storyAssetAllowed(asset, ctx, state, "sinistra") {
					t.Fatal("reviewed public title denied")
				}
				state.Exchanges[1].Text = "我是" + r.KnownAs + "，" + r.RoleRecognition + "。"
				if storyAssetAllowed(asset, ctx, state, "sinistra") {
					t.Fatal("different uncommitted known identity authorized")
				}
				state.Exchanges[1].Text = "我是" + alias + "，" + r.RoleRecognition + "。"
				state.Knowledge[0].RetractedBy = "withdrawn"
				if storyAssetAllowed(asset, ctx, state, "sinistra") {
					t.Fatal("retracted public title observation authorized")
				}
			})
		}
	}
	if count != 3 {
		t.Fatalf("expected exactly three reviewed title rules, got %d", count)
	}
}
