package storypack

import (
	"encoding/json"
	"reflect"
	"testing"
)

// This file also compiles against the pre-fix overlay. The assertions are
// counterexamples there, not skips or expected-failure tests in the fixed suite.
func TestRuleCandidateCreationDeclarationsSurviveAndConstrainOnlyCustomChoices(t *testing.T) {
	for _, raw := range []string{builtinRuleCandidate0, builtinRuleCandidate1, builtinRuleCandidate2, builtinRuleCandidate3, builtinRuleCandidate4, builtinRuleCandidate5} {
		var authored struct {
			ID       string                     `json:"id"`
			Creation map[string]json.RawMessage `json:"creation"`
		}
		if err := json.Unmarshal([]byte(raw), &authored); err != nil {
			t.Fatal(err)
		}
		t.Run(authored.ID, func(t *testing.T) {
			pack, err := NewLibrary(t.TempDir()).Get(authored.ID)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(pack.Creation)
			if err != nil {
				t.Fatal(err)
			}
			var actual map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &actual); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"visible_origin_ids", "custom_state_basis_templates", "custom_location_policy", "custom_background_rule"} {
				var got, want any
				if err := json.Unmarshal(authored.Creation[key], &want); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(actual[key], &got); err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("author declaration disappeared: %s", key)
				}
			}
			var customBases []string
			if err := json.Unmarshal(authored.Creation["custom_state_basis_templates"], &customBases); err != nil {
				t.Fatal(err)
			}
			choice := IdentityChoice{Name: "审核者", Age: pack.Creation.AgeRange[0], OriginID: "custom", StateBasisTemplateID: customBases[0], AffiliationID: "none", LocationID: "auto"}
			for _, templateID := range customBases {
				choice.StateBasisTemplateID = templateID
				if _, err := pack.ResolveIdentity(choice); err != nil {
					t.Fatalf("supported custom basis rejected: %s: %v", templateID, err)
				}
			}
			choice.StateBasisTemplateID = customBases[0]
			choice.LocationText = "未声明的秘密房间"
			if _, err := pack.ResolveIdentity(choice); err == nil {
				t.Fatal("authored_only accepted an undeclared current starting location")
			}
			choice.LocationText = ""
			for _, origin := range pack.Creation.Origins {
				if origin.ID == "custom" {
					continue
				}
				ordinary := choice
				ordinary.OriginID, ordinary.StateBasisTemplateID = origin.ID, ""
				if _, err := pack.ResolveIdentity(ordinary); err != nil {
					t.Fatalf("hidden ordinary origin was deleted or prohibited: %s: %v", origin.ID, err)
				}
			}
			for _, templateID := range pack.Creation.StateBasisTemplates {
				allowed := false
				for _, id := range customBases {
					allowed = allowed || id == templateID
				}
				if allowed {
					continue
				}
				choice.StateBasisTemplateID = templateID
				if _, err := pack.ResolveIdentity(choice); err == nil {
					t.Fatalf("custom origin accepted restricted state basis: %s", templateID)
				}
			}
		})
	}
}

func TestRuleCandidatePreviewPreservesOpaqueInitialFactsWithoutNativeRules(t *testing.T) {
	for _, id := range []string{"doupo-continent-ta07-full-gm-candidate", "harry-potter-1991-ta07-full-gm-candidate", "harry-potter-1991-student-ta07-full-gm-candidate", "lord-of-mysteries-ta07-full-gm-candidate", "sanguo-190-ta07-full-gm-candidate", "zichuan-xichuan-ta07-full-gm-candidate"} {
		t.Run(id, func(t *testing.T) {
			pack, err := NewLibrary(t.TempDir()).Get(id)
			if err != nil {
				t.Fatal(err)
			}
			for _, template := range pack.ProtagonistTemplates {
				initial, err := pack.identityInitialState(template)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(initial)
				if err != nil {
					t.Fatal(err)
				}
				var preview struct {
					Fields []struct {
						Name  string `json:"name"`
						Value any    `json:"value"`
					} `json:"authored_fields"`
				}
				if err := json.Unmarshal(raw, &preview); err != nil {
					t.Fatal(err)
				}
				if len(preview.Fields) != len(template.State) {
					t.Fatalf("%s preview omitted original facts: got %d want %d", template.ID, len(preview.Fields), len(template.State))
				}
				for _, field := range preview.Fields {
					got, _ := json.Marshal(field.Value)
					want, _ := json.Marshal(template.State[field.Name])
					if string(got) != string(want) {
						t.Fatalf("%s preview changed %s", template.ID, field.Name)
					}
				}
				if initial.Family != "" || pack.Story.Gameplay.AdventureRules != nil || pack.Story.Gameplay.CultivationRules != nil {
					t.Fatal("display-only facts enabled a native engine")
				}
			}
		})
	}
}
