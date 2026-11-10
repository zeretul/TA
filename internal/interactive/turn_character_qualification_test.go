package interactive

import (
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
)

func TestTurnCharacterQualificationRequiresFrozenSpeciesMethod(t *testing.T) {
	allowedWorld := &PackCatalogSnapshot{Characters: []PackCatalogPerson{{
		SourceKey: "catalog.spirit-beast", Name: "青风雀",
		Lifeform: PackCatalogLifeformSpiritBeast, CultivationMethodID: "method.spirit_beast_breathing",
	}}}
	profile := TurnCharacterProfileInput{
		CharacterID: "new-beast", Name: "灰团", Lifeform: PackCatalogLifeformSpiritBeast,
		CultivationMethodID: "method.spirit_beast_breathing",
	}
	if diagnostic := validateTurnCharacterQualifications(cultivation.ReferenceRules(),
		&TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{profile}}, nil, allowedWorld); diagnostic != nil {
		t.Fatalf("eligible new spirit beast was rejected: %+v", diagnostic)
	}

	for name, test := range map[string]struct {
		profile TurnCharacterProfileInput
		world   *PackCatalogSnapshot
	}{
		"unknown method": {
			profile: TurnCharacterProfileInput{CharacterID: "new-beast", Name: "灰团", Lifeform: PackCatalogLifeformSpiritBeast, CultivationMethodID: "method.unregistered"},
			world:   allowedWorld,
		},
		"ordinary animal": {
			profile: TurnCharacterProfileInput{CharacterID: "new-animal", Name: "小雀", Lifeform: PackCatalogLifeformAnimal, CultivationMethodID: "method.spirit_beast_breathing"},
			world:   allowedWorld,
		},
		"world without method": {
			profile: profile,
			world:   &PackCatalogSnapshot{Characters: []PackCatalogPerson{{SourceKey: "catalog.person", Name: "掌柜", Lifeform: PackCatalogLifeformHuman}}},
		},
		"borrowed source key": {
			profile: TurnCharacterProfileInput{CharacterID: "new-beast", Name: "灰团", SourceKey: "not-in-world", Lifeform: PackCatalogLifeformSpiritBeast, CultivationMethodID: "method.spirit_beast_breathing"},
			world:   allowedWorld,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if diagnostic := validateTurnCharacterQualifications(cultivation.ReferenceRules(),
				&TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{test.profile}}, nil, test.world); diagnostic == nil {
				t.Fatal("invalid species qualification was accepted")
			}
		})
	}
}

func TestEstablishedProfileCannotBeReclassifiedAfterRegistration(t *testing.T) {
	established := []character.Profile{{ID: "gray", StoryID: "story", Lifeform: PackCatalogLifeformSpiritBeast, CultivationMethodID: "method.spirit_beast_breathing"}}
	for name, profile := range map[string]TurnCharacterProfileInput{
		"change lifeform": {CharacterID: "gray", Name: "灰团", Lifeform: PackCatalogLifeformAnimal},
		"change method":   {CharacterID: "gray", Name: "灰团", Lifeform: PackCatalogLifeformSpiritBeast, CultivationMethodID: "method.other"},
	} {
		t.Run(name, func(t *testing.T) {
			if diagnostic := validateTurnCharacterQualifications(cultivation.ReferenceRules(),
				&TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{profile}}, established, nil); diagnostic == nil {
				t.Fatal("an established identity qualification was changed")
			}
		})
	}
}
