package storypack

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Custom affiliation and location are confirmed character prose, not new world
// catalog entries. They do not grant faction resources, admission or map access.
func (p Pack) identityAffiliation(choice IdentityChoice, origin CreationOrigin, templateID, playerName string) (CreationOption, error) {
	name := strings.TrimSpace(choice.AffiliationText)
	if name == "" {
		return p.resolveAffiliation(choice.AffiliationID, playerName)
	}
	if !validIdentityPlaceName(name, 120) {
		return CreationOption{}, identityError("api.storyPack.identity.customAffiliationInvalid")
	}
	kind := origin.AffiliationKind
	if kind == "" {
		kind = p.affiliationKindForTemplate(templateID)
	}
	if kind != "sect" && kind != "family" {
		return CreationOption{}, identityError("api.storyPack.identity.affiliationNoneForbidden", "origin", origin.Name)
	}
	return CreationOption{ID: fmt.Sprintf("custom.%s.%x", kind, sha256.Sum256([]byte(name))), Name: name, Kind: kind}, nil
}

func (p Pack) identityLocation(choice IdentityChoice, affiliation CreationOption) (CreationOption, error) {
	name := strings.TrimSpace(choice.LocationText)
	if name != "" {
		if !validIdentityPlaceName(name, 200) {
			return CreationOption{}, identityError("api.storyPack.identity.customLocationInvalid")
		}
		if p.Creation != nil && choice.OriginID == "custom" && p.Creation.CustomLocationPolicy == "authored_only" {
			for _, option := range p.Creation.Locations {
				if option.ID != "auto" && (name == strings.TrimSpace(option.Name) || name == option.ID) {
					return p.resolveLocation(option.ID, affiliation)
				}
			}
			return CreationOption{}, identityError("api.storyPack.identity.locationAuthoredOnly")
		}
		return CreationOption{ID: fmt.Sprintf("custom.location.%x", sha256.Sum256([]byte(name))), Name: name}, nil
	}
	id := choice.LocationID
	if (strings.TrimSpace(id) == "" || id == "auto") && p.Creation != nil {
		origin, _ := p.Creation.Origin(choice.OriginID)
		if origin.ID == "custom" && strings.TrimSpace(choice.StateBasisTemplateID) != "" {
			for _, candidate := range p.Creation.Origins {
				if candidate.TemplateID == choice.StateBasisTemplateID && candidate.DefaultLocationID != "" {
					origin = candidate
					break
				}
			}
		}
		if origin.DefaultLocationID != "" {
			id = origin.DefaultLocationID
		}
	}
	if strings.TrimSpace(choice.AffiliationText) != "" {
		id = "auto"
	}
	if strings.TrimSpace(id) == "" || id == "auto" {
		// An affiliation's home is the scene anchor, not another faction's gate.
		if affiliation.Kind == "family" || affiliation.Kind == "sect" {
			return CreationOption{ID: "auto", Name: affiliation.Name + "居处", Country: affiliation.Country}, nil
		}
	}
	return p.resolveLocation(id, affiliation)
}

func validIdentityPlaceName(name string, limit int) bool {
	return utf8.ValidString(name) && utf8.RuneCountInString(name) <= limit && !strings.ContainsFunc(name, unicode.IsControl)
}
