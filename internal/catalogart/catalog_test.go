package catalogart

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func testCatalog(t *testing.T) (fstest.MapFS, manifest) {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buffer.Bytes())
	hash := hex.EncodeToString(sum[:])
	m := manifest{Schema: "catalog-art.v1", Entries: []Entry{{World: "world-a", Kind: "definition", ID: "blade", Version: 1, ReviewState: "reviewed", SourceSHA256: strings.Repeat("a", 64), OriginalSHA256: strings.Repeat("b", 64), Variants: map[string]Variant{"illustration": {SHA256: hash, Width: 2, Height: 2}}}}}
	raw, _ := json.Marshal(m)
	return fstest.MapFS{"assets/manifest.json": {Data: raw}, "assets/images/" + hash + ".png": {Data: buffer.Bytes()}}, m
}

func TestCatalogExactWorldIdentityVersionAndSafeRead(t *testing.T) {
	source, m := testCatalog(t)
	library, err := load(source)
	if err != nil {
		t.Fatal(err)
	}
	hash := m.Entries[0].Variants["illustration"].SHA256
	if got := library.AssetID("world-a", "definition", "blade", 1, "illustration"); got != hash {
		t.Fatalf("asset = %q", got)
	}
	for _, query := range [][4]string{{"world-b", "definition", "blade", "illustration"}, {"world-a", "npc", "blade", "illustration"}, {"world-a", "definition", "unknown", "illustration"}, {"world-a", "definition", "blade", "display"}} {
		if library.URL(query[0], query[1], query[2], 1, query[3]) != "" {
			t.Fatalf("cross-identity asset: %v", query)
		}
	}
	if library.AssetID("world-a", "definition", "blade", 2, "illustration") != "" {
		t.Fatal("version crossed")
	}
	if !library.IsDefinitionAsset(hash) {
		t.Fatal("registered definition missing")
	}
	if _, err := library.Read(hash + ".png"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", hash, hash + ".svg", "../" + hash + ".png", "/" + hash + ".png", "assets/manifest.json", strings.ToUpper(hash) + ".png", hash + ".png?x=1", strings.Repeat("0", 64) + ".png"} {
		if _, err := library.Read(path); err != fs.ErrNotExist {
			t.Fatalf("unsafe path %q: %v", path, err)
		}
	}
}

func TestCatalogMissingCorruptAndDuplicateFailClosed(t *testing.T) {
	for _, mode := range []string{"missing", "corrupt", "dimensions", "duplicate", "unsafe", "unreviewed", "role"} {
		t.Run(mode, func(t *testing.T) {
			source, m := testCatalog(t)
			hash := m.Entries[0].Variants["illustration"].SHA256
			switch mode {
			case "missing":
				delete(source, "assets/images/"+hash+".png")
			case "corrupt":
				source["assets/images/"+hash+".png"] = &fstest.MapFile{Data: []byte("bad bytes")}
			case "dimensions":
				v := m.Entries[0].Variants["illustration"]
				v.Width = 999
				m.Entries[0].Variants["illustration"] = v
			case "duplicate":
				m.Entries = append(m.Entries, m.Entries[0])
			case "unsafe":
				m.Entries[0].ID = "../blade"
			case "unreviewed":
				m.Entries[0].ReviewState = "pending"
			case "role":
				m.Entries[0].Variants["avatar"] = m.Entries[0].Variants["illustration"]
				delete(m.Entries[0].Variants, "illustration")
			}
			raw, _ := json.Marshal(m)
			source["assets/manifest.json"] = &fstest.MapFile{Data: raw}
			l, err := load(source)
			if err == nil {
				t.Fatal("invalid manifest silently accepted")
			}
			if got := l.AssetID("world-a", "definition", "blade", 1, "illustration"); got != "" {
				t.Fatalf("invalid art resolved %s", got)
			}
		})
	}
}

func TestShippedCatalogDecodesAndDoesNotInventDefinitionArt(t *testing.T) {
	library, err := load(assets)
	if err != nil {
		t.Fatal(err)
	}
	for hash := range library.files {
		data, err := library.Read(library.CanonicalFile(hash + ".png"))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
			t.Fatalf("%s: %v", hash, err)
		}
	}
	if id := library.AssetID("lanchuan-qunxia", "npc", "lanchuan-p01", 1, "avatar"); id == "" {
		t.Fatal("pilot portrait missing")
	} else if library.IsDefinitionAsset(id) {
		t.Fatal("portrait accepted as definition artwork")
	}
}
