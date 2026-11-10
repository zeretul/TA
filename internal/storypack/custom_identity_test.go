package storypack

import (
	"strings"
	"testing"
)

func TestCustomAffiliationLocationAndOpeningResolveTogether(t *testing.T) {
	p := fanrenPack(t)
	for _, basis := range []string{"sect-disciple", "family-heir"} {
		choice := IdentityChoice{Name: "叶流云", Gender: "female", Age: 24, OriginID: "custom", OriginText: "从海边归来的游学者", StateBasisTemplateID: basis, AffiliationText: "听潮小筑", LocationText: "海岬旧灯塔", Opening: "落潮时，我在旧灯塔外读一封来信。", RootCategoryID: "dual", RootAttributes: []string{"metal", "water"}, AptitudeID: "ordinary"}
		identity, err := p.ResolveIdentity(choice)
		if err != nil {
			t.Fatal(err)
		}
		if identity.AffiliationName != choice.AffiliationText || identity.LocationName != choice.LocationText || identity.Opening != choice.Opening {
			t.Fatalf("custom choices not preserved: %+v", identity)
		}
		for _, locale := range []string{"zh-CN", "en-US"} {
			shown := p.LocalizeIdentity(identity, locale)
			if shown.OriginText != choice.OriginText || shown.AffiliationName != choice.AffiliationText || shown.LocationName != choice.LocationText || !strings.Contains(shown.Premise, choice.Opening) {
				t.Fatalf("preview replaced custom choices for %s", locale)
			}
		}
		if strings.Contains(identity.Profile, "黄枫谷") || strings.Contains(identity.Profile, "燕家") {
			t.Fatal("custom character inherited optional affiliation")
		}
	}
}
