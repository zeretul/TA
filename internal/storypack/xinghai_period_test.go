package storypack

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestXinghai689ProfilesUseCurrentIdentityAndHistoricalAvailability(t *testing.T) {
	var p Pack
	for _, pack := range adventureContentPacks(t) {
		if pack.ID == "xinghai-689" {
			p = pack
		}
	}
	historical := map[string]bool{"xinghai-p41": true, "xinghai-p42": true, "xinghai-p44": true}
	banned := map[string][]string{
		"xinghai-p13": {"军团高级干部", "Beyond Grade A executive"},
		"xinghai-p14": {"全宇宙最强奶妈", "strongest healer in the universe"},
		"xinghai-p15": {"赤色帝国第二代", "second-generation successor"},
		"xinghai-p18": {"军团的副团长", "Black Star Legion's deputy"},
		"xinghai-p30": {"进化方块前主人", "former owner of the Evolution Cube"},
		"xinghai-p34": {"黑星军团元老", "veteran of the Black Star"},
		"xinghai-p35": {"军团首席暗杀", "chief assassin of the Black Star"},
		"xinghai-p36": {"黑星军团近战", "Black Star Legion's melee"},
		"xinghai-p41": {"圣所复苏归来", "revived through the Sanctum"},
		"xinghai-p42": {"从第三圣所苏醒", "awakened from the Third Sanctum"},
		"xinghai-p44": {"圣约复苏的", "revived by the Holy Accord"},
		"xinghai-p45": {"超A级协会", "Beyond Grade A Association"},
	}
	for _, locale := range []string{"zh-CN", "en-US"} {
		people := p.LocalizedCharacters(locale)
		if len(people) != 60 {
			t.Fatal("era reconciliation removed manuscript people")
		}
		entries := map[string]WorldEntry{}
		for _, entry := range p.LocalizedWorld(locale) {
			entries[entry.ID] = entry
		}
		for _, person := range people {
			entry := entries[person.LoreID]
			if entry.ID == "" {
				t.Fatalf("%s lost its readable profile", person.SourceKey)
			}
			for _, term := range banned[person.SourceKey] {
				if strings.Contains(person.Role+"\n"+person.Brief+"\n"+entry.Content, term) {
					t.Errorf("%s %s contains future identity %q", locale, person.SourceKey, term)
				}
			}
			if person.RecordOnly != historical[person.SourceKey] {
				t.Errorf("%s historical classification changed", person.SourceKey)
			}
			if !person.RecordOnly {
				continue
			}
			if person.PlaceCandidate != "" || person.SpeechStyle != "" {
				t.Errorf("%s gains a living residence or current voice", person.SourceKey)
			}
			if !strings.Contains(entry.Content, "689") {
				t.Errorf("%s single profile lacks its opening-era boundary", person.SourceKey)
			}
			candidates := p.CharacterCandidates(CandidateRequest{Named: []string{person.Name}, Action: person.Name, Location: person.PlaceLabel + " 历史档案 赤色帝国"})
			found := false
			for _, candidate := range candidates {
				if candidate.Character.SourceKey == person.SourceKey {
					found = true
					if candidate.Reach != ReachRemote || !strings.Contains(candidateCard(candidate), "史料") {
						t.Errorf("historical identity became a live candidate: %+v", candidate)
					}
				}
			}
			// English display names need not match the Chinese author's name index.
			if locale == "zh-CN" && !found {
				t.Fatalf("historical profile %s no longer searchable", person.SourceKey)
			}
		}
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordOnlyCatalogFreezeAndCreationEligibility(t *testing.T) {
	p := Pack{ContentVersion: 6, Characters: []CatalogCharacter{{SourceKey: "history", Name: "Ancient", Country: "Here", RecordOnly: true}, {SourceKey: "living", Name: "Present", Country: "Here"}}}
	frozen := p.FreezeCatalog()
	p.Characters[0].RecordOnly = false
	if !frozen.CatalogCharacters()[0].RecordOnly {
		t.Fatal("frozen availability followed author edits")
	}
	encoded, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var restored Frozen
	if err = json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !restored.CatalogCharacters()[0].RecordOnly {
		t.Fatal("freeze roundtrip lost historical boundary")
	}
	p.Characters[0].RecordOnly = true
	choices := p.CreationContactCandidates("", "Here", 6)
	if len(choices) != 1 || choices[0].SourceKey != "living" {
		t.Fatalf("historical record offered as starting acquaintance: %+v", choices)
	}
	bad := p.Characters[0]
	bad.PlaceCandidate = "a-real-place"
	if bad.Validate() == nil {
		t.Fatal("record-only person obtained a living placement")
	}
	bad.PlaceCandidate = ""
	bad.MembershipAdmission = []string{"formal"}
	bad.FactionID = "guild"
	bad.FactionName = "Guild"
	if bad.Validate() == nil {
		t.Fatal("record-only person obtained admission authority")
	}
	var legacy Frozen
	if err = json.Unmarshal([]byte(`{"People":[{"SourceKey":"history","Name":"Ancient"}]}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.CatalogCharacters()[0].RecordOnly {
		t.Fatal("legacy snapshot upgraded from current catalog")
	}
}
