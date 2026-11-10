package generation

import "testing"

// A run with no cast encodes to the empty string, which is what a config field
// that was never set holds. Decoding it back must not invent a person.
func TestEncodeReferencePeopleEmptyRoundTrips(t *testing.T) {
	if got := EncodeReferencePeople(nil); got != "" {
		t.Fatalf("nil cast encoded as %q", got)
	}
	if got := EncodeReferencePeople([]ReferencePerson{}); got != "" {
		t.Fatalf("empty cast encoded as %q", got)
	}
	if people := DecodeReferencePeople(""); len(people) != 0 {
		t.Fatalf("empty string decoded to %d people", len(people))
	}
}

func TestReferencePeopleRoundTripKeepsEveryField(t *testing.T) {
	cast := []ReferencePerson{
		{Name: "陈巧倩", SourceKey: "canon_chen_qiaoqian", Appearance: "五官清秀。", Picture: ReferencePicture(PictureShipped, "canon_chen_qiaoqian")},
		{Name: "韩掌柜", Picture: ReferencePicture(PictureInMedia, "ab12.png")},
		{Name: "柳三", Picture: ReferencePicture(PictureInWorkspace, "assets/avatars/liu.png")},
	}
	decoded := DecodeReferencePeople(EncodeReferencePeople(cast))
	if len(decoded) != len(cast) {
		t.Fatalf("expected %d people, got %d", len(cast), len(decoded))
	}
	for index := range cast {
		if decoded[index] != cast[index] {
			t.Fatalf("person %d changed: %#v != %#v", index, decoded[index], cast[index])
		}
	}
}

// A run record is rebuilt from JSON after a restart, and a truncated or
// hand-edited one must not take the image path down with it.
func TestDecodeReferencePeopleIgnoresMalformedInput(t *testing.T) {
	for _, raw := range []string{"{", "not json", "null", `{"name":"x"}`} {
		if people := DecodeReferencePeople(raw); len(people) != 0 {
			t.Fatalf("%q decoded to %d people", raw, len(people))
		}
	}
}

// An entry with no name cannot label a face, so it is dropped rather than sent
// with an empty label the model would have to guess at.
func TestDecodeReferencePeopleDropsNamelessEntries(t *testing.T) {
	raw := EncodeReferencePeople([]ReferencePerson{
		{Name: "   ", Picture: ReferencePicture(PictureShipped, "canon_chen_qiaoqian")},
		{Name: "陈巧倩", Picture: ReferencePicture(PictureShipped, "canon_chen_qiaoqian")},
	})
	people := DecodeReferencePeople(raw)
	if len(people) != 1 || people[0].Name != "陈巧倩" {
		t.Fatalf("nameless entry survived: %#v", people)
	}
}

func TestSplitReferencePictureRecognisesEachSource(t *testing.T) {
	cases := []struct {
		locator string
		source  string
		name    string
	}{
		{PictureShipped + "canon_chen_qiaoqian", PictureShipped, "canon_chen_qiaoqian"},
		{PictureInMedia + "ab12.png", PictureInMedia, "ab12.png"},
		{PictureInWorkspace + "assets/a.png", PictureInWorkspace, "assets/a.png"},
		// A locator written before the prefixes existed was a workspace path.
		{"assets/a.png", PictureInWorkspace, "assets/a.png"},
		{"", PictureInWorkspace, ""},
	}
	for _, testCase := range cases {
		source, name := SplitReferencePicture(testCase.locator)
		if source != testCase.source || name != testCase.name {
			t.Fatalf("%q split to (%q, %q), want (%q, %q)", testCase.locator, source, name, testCase.source, testCase.name)
		}
	}
}

func TestReferencePictureRefusesEmptyName(t *testing.T) {
	if got := ReferencePicture(PictureShipped, "   "); got != "" {
		t.Fatalf("empty name produced locator %q", got)
	}
}
