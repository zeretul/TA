package storypack

import (
	"encoding/json"
	"os"
	"testing"
)

// The optional output is a rendering fixture built by the same creation
// projections as the product. It does not call a model or create a player save.
func TestFantasyExportBrowserFixture(t *testing.T) {
	path := os.Getenv("TA_FANTASY_FIXTURE_OUTPUT")
	if path == "" {
		t.Skip("optional browser fixture export")
	}
	root := t.TempDir()
	p, err := NewLibrary(root).Get("sui-guan-realms")
	if err != nil {
		t.Fatal(err)
	}
	locales := map[string]any{}
	for _, locale := range []string{"zh-CN", "en-US"} {
		identities := map[string]Identity{}
		for _, origin := range p.Creation.Origins {
			identity, e := p.ResolveIdentity(IdentityChoice{Name: "Rowan", Age: 24, OriginID: origin.ID})
			if e != nil {
				t.Fatal(e)
			}
			identities[origin.ID] = p.LocalizeIdentity(identity, locale)
		}
		locales[locale] = map[string]any{"summary": p.DescribeLocale(root, locale), "identities": identities}
	}
	frozen, err := p.FrozenGameplay()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(map[string]any{"locales": locales, "rules": frozen.AdventureRules, "templates": p.ProtagonistTemplates}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
