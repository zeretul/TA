package storypack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The creation page can refuse a set of choices, and the page shows that refusal
// in whatever language it is reading. These tests pin the whole vocabulary: every
// key the code can raise has both languages, the English column never carries a
// Chinese character, and the same refusal reads differently in the two languages.

// TestEveryRefusalHasBothLanguages is the table's own invariant. A sentence added
// in one language only would silently fall back to Chinese on an English page.
func TestEveryRefusalHasBothLanguages(t *testing.T) {
	if len(identityMessages) == 0 {
		t.Fatal("the message table is empty; the test would prove nothing")
	}
	for key, pair := range identityMessages {
		if strings.TrimSpace(pair[0]) == "" {
			t.Errorf("%s has no Chinese text", key)
		}
		if strings.TrimSpace(pair[1]) == "" {
			t.Errorf("%s has no English text", key)
		}
		if count := hanCount(pair[1]); count > 0 {
			t.Errorf("%s carries %d Chinese characters in its English column: %q", key, count, pair[1])
		}
		if strings.Contains(pair[1], "{{") != strings.Contains(pair[0], "{{") {
			t.Errorf("%s interpolates a value in one language but not the other: %q / %q", key, pair[0], pair[1])
		}
	}
}

// keyedErrorPattern matches the two shapes a keyed refusal is written in: the
// package's own factory, and the struct literal the app layer builds.
var keyedErrorPattern = regexp.MustCompile(`identityError\("([^"]+)"|IdentityError\{Key: "([^"]+)"`)

// TestNoRefusalKeyIsMisspelled reads the package source for the keys it raises
// and checks each one against the table. A typo there is invisible at compile
// time and would only surface as a raw key shown to a player.
func TestNoRefusalKeyIsMisspelled(t *testing.T) {
	dirs := []string{".", filepath.Join("..", "app")}
	found := 0
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			source, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			for _, match := range keyedErrorPattern.FindAllStringSubmatch(string(source), -1) {
				key := match[1]
				if key == "" {
					key = match[2]
				}
				found++
				if _, ok := identityMessages[key]; !ok {
					t.Errorf("%s raises %s, which the message table does not hold", name, key)
				}
			}
		}
	}
	if found == 0 {
		t.Fatal("no keyed refusal was found in the source; the pattern is out of date")
	}
}

// TestARefusalReadsInThePagesLanguage walks the sentences a player is most likely
// to hit. Each must differ between the two languages and leave no placeholder
// behind, because an unfilled {{max}} is worse than a wrong language.
func TestARefusalReadsInThePagesLanguage(t *testing.T) {
	refusals := []error{
		identityError("api.storyPack.identity.nameRequired"),
		identityError("api.storyPack.identity.nameTooLong", "max", identityNameMaxRunes),
		identityError("api.storyPack.identity.ageOutOfRange", "age", 7, "min", 16, "max", 60),
		identityError("api.storyPack.identity.rootArityExact", "category", "Single", "count", 1, "chosen", 2),
		identityError("api.storyPack.identity.traitTooMany", "max", identityMaxTraits),
		identityError("api.storyPack.templateMissing", "template", "lin-zhao"),
	}
	for _, refusal := range refusals {
		zh := IdentityErrorText(refusal, "zh-CN")
		en := IdentityErrorText(refusal, "en-US")
		if zh == "" || en == "" {
			t.Fatalf("%v wrote an empty sentence: %q / %q", refusal, zh, en)
		}
		if zh == en {
			t.Fatalf("%v reads the same in both languages: %q", refusal, zh)
		}
		if hanCount(zh) == 0 {
			t.Fatalf("%v lost its Chinese wording: %q", refusal, zh)
		}
		if count := hanCount(en); count > 0 {
			t.Fatalf("%v carries %d Chinese characters in English: %q", refusal, count, en)
		}
		for _, text := range []string{zh, en} {
			if strings.Contains(text, "{{") {
				t.Fatalf("%v left a placeholder unfilled: %q", refusal, text)
			}
		}
	}
}

// TestAnUnknownLocaleFallsBackToChinese keeps the default language stable. A
// request with no locale header must read the same as the Chinese page.
func TestAnUnknownLocaleFallsBackToChinese(t *testing.T) {
	refusal := identityError("api.storyPack.identity.nameRequired")
	zh := IdentityErrorText(refusal, "zh-CN")
	for _, locale := range []string{"", "  ", "fr-FR", "zh-TW", "ZH-CN"} {
		if got := IdentityErrorText(refusal, locale); got != zh {
			t.Fatalf("locale %q read %q, want the Chinese sentence %q", locale, got, zh)
		}
	}
	if got := IdentityErrorText(refusal, "EN-us"); got == zh {
		t.Fatal("an upper-case English locale was not recognised")
	}
}

// TestAFailureThePackDidNotWriteIsPassedThrough stops a wrapper from inventing a
// sentence for a failure it does not understand.
func TestAFailureThePackDidNotWriteIsPassedThrough(t *testing.T) {
	technical := fmt.Errorf("open %s: permission denied", "/tmp/world.json")
	if got := IdentityErrorText(technical, "en-US"); got != technical.Error() {
		t.Fatalf("a foreign failure was rewritten: %q", got)
	}
	if got := IdentityErrorText(nil, "en-US"); got != "" {
		t.Fatalf("a nil failure wrote %q", got)
	}
	// Wrapping must not lose the keyed sentence, which is why the type is read
	// with errors.As rather than a type assertion.
	wrapped := fmt.Errorf("drafting: %w", identityError("api.storyPack.identity.nameRequired"))
	if got := IdentityErrorText(wrapped, "en-US"); got != "Enter a name." {
		t.Fatalf("a wrapped refusal read %q", got)
	}
}

// TestResolvedValidationRefusesInBothLanguages is the end-to-end half: real
// choices the pack rejects, resolved through the real entry point, read in both
// languages. It is what a page actually gets back from the API.
func TestResolvedValidationRefusesInBothLanguages(t *testing.T) {
	pack := fanrenPack(t)
	valid := englishChoice()

	cases := []struct {
		name  string
		edit  func(*IdentityChoice)
		key   string
		zhHas string
		enHas string
	}{
		{"empty name", func(c *IdentityChoice) { c.Name = "  " },
			"api.storyPack.identity.nameRequired", "请填写姓名", "Enter a name."},
		{"long name", func(c *IdentityChoice) { c.Name = strings.Repeat("Lin", 20) },
			"api.storyPack.identity.nameTooLong", "姓名过长", "too long"},
		{"unknown origin", func(c *IdentityChoice) { c.OriginID = "no.such.origin" },
			"api.storyPack.identity.originUnknown", "未知出身", "Unknown origin"},
		{"unknown location", func(c *IdentityChoice) { c.LocationID = "no.such.place" },
			"api.storyPack.identity.locationUnknown", "未知落脚地点", "Unknown starting place"},
		{"too many traits", func(c *IdentityChoice) {
			c.TraitIDs = []string{"steady", "curious", "guarded", "softhearted"}
		}, "api.storyPack.identity.traitTooMany", "特性最多选", "at most"},
	}
	for _, check := range cases {
		choice := valid
		check.edit(&choice)
		_, err := pack.ResolveIdentity(choice)
		if err == nil {
			t.Fatalf("%s: the pack accepted choices it should refuse", check.name)
		}
		var keyed *IdentityError
		if !errors.As(err, &keyed) {
			t.Fatalf("%s: the refusal is not keyed, so the page cannot translate it: %v", check.name, err)
		}
		if keyed.Key != check.key {
			t.Fatalf("%s: refused with %s, want %s", check.name, keyed.Key, check.key)
		}
		zh := IdentityErrorText(err, "zh-CN")
		en := IdentityErrorText(err, "en-US")
		if !strings.Contains(zh, check.zhHas) {
			t.Fatalf("%s: the Chinese sentence lost its wording: %q", check.name, zh)
		}
		if !strings.Contains(en, check.enHas) {
			t.Fatalf("%s: the English sentence lost its wording: %q", check.name, en)
		}
		if count := hanCount(en); count > 0 {
			t.Fatalf("%s: the English sentence carries %d Chinese characters: %q", check.name, count, en)
		}
	}
}
