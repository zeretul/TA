package lore

import "testing"

// The world-creation prompt used to ask the model for types the reader does not
// know: setting, history and custom. Every world written before that changed
// still carries them, so they arrive on the read path, and until now all three
// were filed as 其他 — the same drawer as the protagonist templates, next to the
// items rather than with the world they describe. A player opening 资料总览 saw
// the world's own geography and rules under a heading that says nothing.
//
// The prompt now asks for the real types, so new worlds need no conversion. This
// covers the worlds that already exist and are still being played.
func TestLegacyWorldEntryTypesAreReadable(t *testing.T) {
	cases := map[string]string{
		"setting": "world",
		"history": "world",
		// A type nothing defines still has to land somewhere; 其他 is that
		// answer, and it stays the answer for anything genuinely unknown.
		"custom": "other",
	}
	for input, want := range cases {
		if got := NormalizeType(input); got != want {
			t.Errorf("NormalizeType(%q) = %q, want %q", input, got, want)
		}
	}

	// The label is what the player actually reads, so the conversion has to
	// reach it rather than stopping at the stored value.
	if got := TypeLabel("setting"); got != "世界观" {
		t.Errorf("TypeLabel(setting) = %q, want 世界观", got)
	}
	if got := TypeLabel("history"); got != "世界观" {
		t.Errorf("TypeLabel(history) = %q, want 世界观", got)
	}
}

// A legacy entry written by the old prompt is created through the ordinary
// store, which normalizes on write. What is written and what is read back must
// agree, otherwise the editor would show a type the list does not group under.
func TestALegacyTypeIsStoredAsTheTypeTheListShows(t *testing.T) {
	store := NewStore(t.TempDir())
	item, err := store.Create(ItemInput{
		ID: "han-shan-yi", Type: "setting", Name: "寒山驿",
		Content: "驿镇夹在两山之间，入冬后官道断绝。",
	})
	if err != nil {
		t.Fatalf("creating a legacy-typed entry failed: %v", err)
	}
	if item.Type != "world" {
		t.Fatalf("stored type = %q, want world", item.Type)
	}
	read, err := store.Get("han-shan-yi")
	if err != nil {
		t.Fatalf("reading the entry back failed: %v", err)
	}
	if read.Type != "world" {
		t.Fatalf("read type = %q, want world", read.Type)
	}
}
