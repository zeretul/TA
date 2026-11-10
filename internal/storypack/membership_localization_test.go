package storypack

import (
	"reflect"
	"testing"
)

func TestAdmissionFactionEnglishDisplayPreservesFrozenAuthority(t *testing.T) {
	for _, tc := range []struct{ pack, source, english string }{
		{"fanren-jie", "leader_zhong_lingdao", "Huangfeng Valley"},
		{"lanchuan-qunxia", "lanchuan-p11", "Surging Wave Boat Gang"},
		{"xinghai-689", "xinghai-p01", "Black Star Mercenary Group"},
	} {
		t.Run(tc.pack, func(t *testing.T) {
			pack, err := NewLibrary(t.TempDir()).Get(tc.pack)
			if err != nil {
				t.Fatal(err)
			}
			var source CatalogCharacter
			for _, p := range pack.Characters {
				if p.SourceKey == tc.source {
					source = p
				}
			}
			if source.FactionID == "" || source.FactionName == "" || len(source.MembershipAdmission) != 2 {
				t.Fatal("missing canonical admission authority", source)
			}
			var translated CatalogCharacter
			for _, p := range pack.localizedCharacters("en-US") {
				if p.SourceKey == tc.source {
					translated = p
				}
			}
			if translated.FactionName != tc.english {
				t.Fatal("faction name was not localized", translated.FactionName)
			}
			if translated.SourceKey != source.SourceKey || translated.FactionID != source.FactionID || !reflect.DeepEqual(translated.MembershipAdmission, source.MembershipAdmission) {
				t.Fatal("display localization changed admission authority")
			}
			for _, p := range pack.Characters {
				if p.SourceKey == tc.source && !reflect.DeepEqual(p, source) {
					t.Fatal("localization mutated canonical catalog")
				}
			}
		})
	}
}
