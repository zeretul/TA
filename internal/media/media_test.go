package media

import (
	"os"
	"path/filepath"
	"testing"
)

// writeManifest lays down a manifest and the files it names, so the library is
// exercised the way a shipped directory is.
func writeManifest(t *testing.T, entries string) string {
	t.Helper()
	dir := t.TempDir()
	document := `{"schema":"` + ManifestSchema + `","entries":[` + entries + `]}`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(document), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return dir
}

func writeClip(t *testing.T, dir, name string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write clip %s: %v", name, err)
	}
}

func TestAMissingManifestLeavesTheBuildWithoutClips(t *testing.T) {
	library := NewLibrary(t.TempDir())
	if clips := library.Clips("npc_ning_fuling"); len(clips) != 0 {
		t.Fatalf("expected no clips without a manifest, got %d", len(clips))
	}
	if _, ok := library.General("npc_ning_fuling"); ok {
		t.Fatal("expected no general clip without a manifest")
	}
}

func TestAnEmptyDirectoryIsNotAnError(t *testing.T) {
	// A build that ships no clips at all passes "". It must behave the same as a
	// directory with no manifest: the game starts and the window keeps its
	// portrait.
	library := NewLibrary("")
	if clips := library.Clips("npc_ning_fuling"); len(clips) != 0 {
		t.Fatalf("expected no clips, got %d", len(clips))
	}
}

func TestClipsAreKeyedByTheAuthorSourceKey(t *testing.T) {
	dir := writeManifest(t, `
		{"source_key":"npc_ning_fuling","file":"aaaaaaaaaaaaaaaa.mp4","sha256":"aa","bytes":3,"duration_ms":5000,"width":640,"height":640,"role":"general"},
		{"source_key":"npc_ning_fuling","file":"bbbbbbbbbbbbbbbb.mp4","sha256":"bb","bytes":3,"duration_ms":5000,"width":640,"height":640,"role":"intimate"}
	`)
	writeClip(t, dir, "aaaaaaaaaaaaaaaa.mp4", "aaa")
	writeClip(t, dir, "bbbbbbbbbbbbbbbb.mp4", "bbb")
	library := NewLibrary(dir)

	clips := library.Clips("npc_ning_fuling")
	if len(clips) != 2 {
		t.Fatalf("expected 2 clips, got %d", len(clips))
	}
	// General sorts before intimate, so an ordinary surface never has to look at
	// the private set to find the one it is entitled to.
	if clips[0].Role != RoleGeneral {
		t.Fatalf("expected the general clip first, got %q", clips[0].Role)
	}
	general, ok := library.General("npc_ning_fuling")
	if !ok || general.File != "aaaaaaaaaaaaaaaa.mp4" {
		t.Fatalf("expected the general clip, got %+v ok=%v", general, ok)
	}
	intimate, ok := library.Intimate("npc_ning_fuling")
	if !ok || intimate.File != "bbbbbbbbbbbbbbbb.mp4" {
		t.Fatalf("expected the intimate clip, got %+v ok=%v", intimate, ok)
	}
}

func TestAPersonWithOnlyOneRoleHasNoClipOfTheOther(t *testing.T) {
	dir := writeManifest(t, `
		{"source_key":"npc_han_li","file":"cccccccccccccccc.mp4","sha256":"cc","bytes":3,"duration_ms":5000,"width":640,"height":640,"role":"intimate"}
	`)
	writeClip(t, dir, "cccccccccccccccc.mp4", "ccc")
	library := NewLibrary(dir)

	if _, ok := library.General("npc_han_li"); ok {
		t.Fatal("a person with only a private clip has no ordinary-day clip")
	}
	if _, ok := library.Intimate("npc_han_li"); !ok {
		t.Fatal("expected the private clip to be found")
	}
}

func TestAnUnknownKeyReturnsNothing(t *testing.T) {
	dir := writeManifest(t, `
		{"source_key":"npc_ning_fuling","file":"aaaaaaaaaaaaaaaa.mp4","sha256":"aa","bytes":3,"duration_ms":5000,"width":640,"height":640,"role":"general"}
	`)
	writeClip(t, dir, "aaaaaaaaaaaaaaaa.mp4", "aaa")
	library := NewLibrary(dir)

	if clips := library.Clips("npc_nobody"); len(clips) != 0 {
		t.Fatalf("expected nothing for an unknown key, got %d", len(clips))
	}
	if clips := library.Clips(""); len(clips) != 0 {
		t.Fatalf("expected nothing for an empty key, got %d", len(clips))
	}
}

func TestTheRouteOnlyOpensFilesTheManifestNames(t *testing.T) {
	dir := writeManifest(t, `
		{"reviewed":true,"source_key":"npc_ning_fuling","file":"aaaaaaaaaaaaaaaa.mp4","sha256":"aa","bytes":3,"duration_ms":5000,"width":640,"height":640,"role":"general"}
	`)
	writeClip(t, dir, "aaaaaaaaaaaaaaaa.mp4", "aaa")
	// A file that exists on disk but that the manifest does not name must stay
	// unreadable: the route is a lookup, not a directory read.
	writeClip(t, dir, "not-listed.mp4", "secret")
	library := NewLibrary(dir)

	handle, clip, err := library.Open("aaaaaaaaaaaaaaaa.mp4")
	if err != nil {
		t.Fatalf("expected the listed clip to open: %v", err)
	}
	_ = handle.Close()
	if clip.File != "aaaaaaaaaaaaaaaa.mp4" {
		t.Fatalf("expected the listed clip, got %+v", clip)
	}

	for _, name := range []string{"not-listed.mp4", "../media.json", "..", ".", "", "sub/aaaaaaaaaaaaaaaa.mp4", `sub\aaaaaaaaaaaaaaaa.mp4`} {
		if _, _, err := library.Open(name); err == nil {
			t.Fatalf("expected %q to be refused", name)
		}
	}
}

func TestAManifestEntryThatEscapesTheDirectoryIsDropped(t *testing.T) {
	// A manifest is data that ships with the build, but a separator in a file
	// name would still let a request read outside the media directory, so such an
	// entry is discarded when the library loads rather than trusted at request
	// time.
	dir := writeManifest(t, `
		{"source_key":"npc_ning_fuling","file":"../outside.mp4","sha256":"aa","bytes":3,"duration_ms":5000,"width":640,"height":640,"role":"general"},
		{"source_key":"../npc_ning_fuling","file":"aaaaaaaaaaaaaaaa.mp4","sha256":"bb","bytes":3,"duration_ms":5000,"width":640,"height":640,"role":"general"}
	`)
	library := NewLibrary(dir)

	if clips := library.Clips("npc_ning_fuling"); len(clips) != 0 {
		t.Fatalf("expected the escaping file entry to be dropped, got %d", len(clips))
	}
	if _, _, err := library.Open("../outside.mp4"); err == nil {
		t.Fatal("expected the escaping file name to be refused")
	}
}

func TestPickIsStableForASceneAndCoversTheSet(t *testing.T) {
	clips := []Asset{{File: "a.mp4"}, {File: "b.mp4"}, {File: "c.mp4"}}

	first, ok := Pick(clips, "node-1|越国坊市")
	if !ok {
		t.Fatal("expected a clip")
	}
	// The same scene has to show the same clip when the window is reopened and
	// after a cold restart, so the choice cannot depend on anything but the seed.
	for i := 0; i < 20; i++ {
		again, _ := Pick(clips, "node-1|越国坊市")
		if again.File != first.File {
			t.Fatalf("pick changed between calls: %q then %q", first.File, again.File)
		}
	}

	seen := map[string]bool{}
	for _, seed := range []string{"node-1|越国坊市", "node-2|越国坊市", "node-3|越国坊市", "node-4|黄枫谷", "node-5|黄枫谷", "node-6|黄枫谷"} {
		clip, _ := Pick(clips, seed)
		seen[clip.File] = true
	}
	if len(seen) < 2 {
		t.Fatalf("expected different scenes to reach different clips, saw only %v", seen)
	}
}

func TestPickOfOneClipAlwaysAnswersIt(t *testing.T) {
	clip, ok := Pick([]Asset{{File: "only.mp4"}}, "anything")
	if !ok || clip.File != "only.mp4" {
		t.Fatalf("expected the single clip, got %+v ok=%v", clip, ok)
	}
	if _, ok := Pick(nil, "anything"); ok {
		t.Fatal("expected no clip from an empty set")
	}
}

func TestURLPointsAtTheMediaRoute(t *testing.T) {
	if got := URL(Asset{File: "aaaaaaaaaaaaaaaa.mp4"}); got != "/api/media/aaaaaaaaaaaaaaaa.mp4" {
		t.Fatalf("unexpected URL %q", got)
	}
	if got := URL(Asset{}); got != "" {
		t.Fatalf("expected no URL for an empty clip, got %q", got)
	}
}
