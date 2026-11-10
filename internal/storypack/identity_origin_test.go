package storypack

import (
	"errors"
	"strings"
	"testing"
)

// The creation page omits origin_text for authored choices. Long authored
// descriptions must survive both draft validation and confirmation intact;
// the custom-input limit must not reject the server's own fallback copy.
func TestResolveIdentityAcceptsAllAdventureAuthoredOrigins(t *testing.T) {
	for _, pack := range adventureContentPacks(t) {
		for _, origin := range pack.Creation.Origins {
			t.Run(pack.ID+"/"+origin.ID, func(t *testing.T) {
				for _, input := range []string{"", " \n\t "} {
					identity, err := pack.ResolveIdentity(IdentityChoice{
						Name: "云端验收甲", Age: 24, OriginID: origin.ID, OriginText: input,
					})
					if err != nil {
						t.Fatalf("resolve authored origin: %v", err)
					}
					want := applyOriginPlaceholders(origin.Description, identity.AffiliationName)
					if identity.OriginText != want {
						t.Fatalf("authored origin was changed or truncated: got %q, want %q", identity.OriginText, want)
					}
					if !strings.Contains(identity.Profile, want) || !strings.Contains(identity.Premise, want) {
						t.Fatal("authored origin did not reach the confirmed character prose")
					}
				}
			})
		}
	}
}

func TestResolveIdentityBoundsOnlyPlayerOriginInput(t *testing.T) {
	pack := fanrenPack(t)
	for _, originID := range []string{"custom", "loose"} {
		for _, size := range []int{120, 121} {
			choice := ageChoice(24)
			choice.OriginID = originID
			choice.StateBasisTemplateID = "loose-cultivator"
			choice.OriginText = " \n" + strings.Repeat("云", size) + "\t "
			identity, err := pack.ResolveIdentity(choice)
			if size == 120 {
				if err != nil {
					t.Fatalf("%s: 120-rune player text was refused: %v", originID, err)
				}
				if identity.OriginText != strings.Repeat("云", size) {
					t.Fatalf("%s: player text was not preserved", originID)
				}
				continue
			}
			var refusal *IdentityError
			if !errors.As(err, &refusal) || refusal.Key != "api.storyPack.identity.originTooLong" {
				t.Fatalf("%s: 121-rune player text error = %v, want originTooLong", originID, err)
			}
		}
	}
}
