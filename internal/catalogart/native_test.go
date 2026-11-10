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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func nativeFixture(t *testing.T) (fstest.MapFS, manifest, []byte) {
	t.Helper()
	original, err := os.ReadFile("testdata/native-alpha.png")
	if err != nil {
		t.Fatal(err)
	}
	compressed, err := os.ReadFile("testdata/native-alpha.webp")
	if err != nil {
		t.Fatal(err)
	}
	originalSum, compressedSum := sha256.Sum256(original), sha256.Sum256(compressed)
	_, m := testCatalog(t)
	v := Variant{SHA256: hex.EncodeToString(compressedSum[:]), Width: 8, Height: 8, Format: "webp", LegacySHA256: hex.EncodeToString(originalSum[:])}
	m.Entries[0].Variants["illustration"] = v
	raw, _ := json.Marshal(m)
	return fstest.MapFS{"assets/manifest.json": {Data: raw}, "assets/images/" + v.SHA256 + ".webp": {Data: compressed}}, m, original
}

func compareNativePixels(t *testing.T, original, native []byte) {
	t.Helper()
	a, err := png.Decode(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	b, format, err := image.Decode(bytes.NewReader(native))
	if err != nil {
		t.Fatal(err)
	}
	if format != "webp" || a.Bounds() != b.Bounds() {
		t.Fatal("Native dimensions or format changed")
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if color.NRGBAModel.Convert(a.At(x, y)).(color.NRGBA) != color.NRGBAModel.Convert(b.At(x, y)).(color.NRGBA) {
				t.Fatalf("RGBA mismatch at %d,%d", x, y)
			}
		}
	}
}

func TestNativeWebPHashMIMEAliasesAndAlpha(t *testing.T) {
	source, m, original := nativeFixture(t)
	library, err := load(source)
	if err != nil {
		t.Fatal(err)
	}
	v := m.Entries[0].Variants["illustration"]
	canonical := v.SHA256 + ".webp"
	if library.URL("world-a", "definition", "blade", 1, "illustration") != PathPrefix+canonical {
		t.Fatal("Canonical URL differs")
	}
	for _, spelling := range []string{canonical, v.SHA256 + ".png", v.LegacySHA256 + ".png"} {
		if got := library.CanonicalFile(spelling); got != canonical {
			t.Fatalf("Legacy URL unresolved: %s", spelling)
		}
	}
	for _, id := range []string{v.SHA256, v.LegacySHA256} {
		if !library.IsDefinitionAsset(id) {
			t.Fatal("Saved definition ID lost")
		}
	}
	data, err := library.Read(canonical)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(data)
	if hex.EncodeToString(h[:]) != v.SHA256 {
		t.Fatal("Canonical URL hash does not match response bytes")
	}
	if library.ContentType(canonical) != "image/webp" {
		t.Fatal("Wrong WebP MIME")
	}
	compareNativePixels(t, original, data)
	for _, file := range []string{v.SHA256 + ".png", v.LegacySHA256 + ".png", v.LegacySHA256 + ".webp", "../" + canonical} {
		if _, err := library.Read(file); err != fs.ErrNotExist {
			t.Fatal("Noncanonical URL served bytes directly")
		}
	}
}

func TestNativeWebPAndOriginalPNGCoexist(t *testing.T) {
	source, m, _ := nativeFixture(t)
	pngSource, pngManifest := testCatalog(t)
	pngManifest.Entries[0].World = "world-png"
	m.Entries = append(m.Entries, pngManifest.Entries[0])
	for name, file := range pngSource {
		if name != "assets/manifest.json" {
			source[name] = file
		}
	}
	raw, _ := json.Marshal(m)
	source["assets/manifest.json"] = &fstest.MapFile{Data: raw}
	library, err := load(source)
	if err != nil {
		t.Fatal(err)
	}
	id := pngManifest.Entries[0].Variants["illustration"].SHA256
	if library.URL("world-png", "definition", "blade", 1, "illustration") != PathPrefix+id+".png" {
		t.Fatal("Original PNG URL changed")
	}
	data, err := library.Read(id + ".png")
	if err != nil || !bytes.Equal(data, pngSource["assets/images/"+id+".png"].Data) || library.ContentType(id+".png") != "image/png" {
		t.Fatal("Original PNG bytes or MIME changed")
	}
	// An alias is never allowed to shadow an existing byte-content identity.
	v := m.Entries[0].Variants["illustration"]
	v.LegacySHA256 = id
	m.Entries[0].Variants["illustration"] = v
	raw, _ = json.Marshal(m)
	source["assets/manifest.json"] = &fstest.MapFile{Data: raw}
	if _, err := load(source); err == nil {
		t.Fatal("Alias shadowed current PNG hash")
	}
}

func TestRetainedHistoricalArtSurvivesAuthoringReplacement(t *testing.T) {
	source, m, original := nativeFixture(t)
	v := m.Entries[0].Variants["illustration"]
	legacy := v.LegacySHA256
	v.LegacySHA256 = ""
	m.Entries = nil
	m.RetainedImages = []retainedImage{{Variant: v, Definition: true}}
	m.Aliases = map[string]string{legacy: v.SHA256}
	raw, _ := json.Marshal(m)
	source["assets/manifest.json"] = &fstest.MapFile{Data: raw}
	library, err := load(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(library.EntriesForAudit()) != 0 {
		t.Fatal("Historical art invented a current author entry")
	}
	if !library.IsDefinitionAsset(legacy) || library.CanonicalFile(legacy+".png") != v.SHA256+".webp" {
		t.Fatal("Frozen story reference lost")
	}
	data, err := library.Read(v.SHA256 + ".webp")
	if err != nil {
		t.Fatal(err)
	}
	compareNativePixels(t, original, data)
	m.Aliases[legacy] = strings.Repeat("0", 64)
	raw, _ = json.Marshal(m)
	source["assets/manifest.json"] = &fstest.MapFile{Data: raw}
	if _, err := load(source); err == nil {
		t.Fatal("Dangling historical URL accepted")
	}
}

func TestNativeWebPRejectsBadFormatsHashesAliasesAndLossyPayload(t *testing.T) {
	for _, mode := range []string{"format", "hash", "legacy", "missing", "corrupt", "lossy", "dimensions", "alias-conflict"} {
		t.Run(mode, func(t *testing.T) {
			source, m, _ := nativeFixture(t)
			v := m.Entries[0].Variants["illustration"]
			path := "assets/images/" + v.SHA256 + ".webp"
			switch mode {
			case "format":
				v.Format = "svg"
			case "hash":
				v.SHA256 = strings.Repeat("0", 64)
			case "legacy":
				v.LegacySHA256 = "../invalid"
			case "missing":
				delete(source, path)
			case "corrupt":
				source[path] = &fstest.MapFile{Data: []byte("bad")}
			case "lossy":
				data, err := os.ReadFile("testdata/native-lossy.webp")
				if err != nil {
					t.Fatal(err)
				}
				h := sha256.Sum256(data)
				v.SHA256 = hex.EncodeToString(h[:])
				source["assets/images/"+v.SHA256+".webp"] = &fstest.MapFile{Data: data}
			case "dimensions":
				v.Width = 7
			case "alias-conflict":
				v.LegacySHA256 = v.SHA256
			}
			m.Entries[0].Variants["illustration"] = v
			raw, _ := json.Marshal(m)
			source["assets/manifest.json"] = &fstest.MapFile{Data: raw}
			l, err := load(source)
			if err == nil || l.AssetID("world-a", "definition", "blade", 1, "illustration") != "" {
				t.Fatal("Invalid native image accepted")
			}
		})
	}
}

func TestNativeOriginalPixelsAndContentHashes(t *testing.T) {
	root := os.Getenv("TA_CATALOG_ORIGINAL_DIR")
	if root == "" {
		t.Skip("Original PNGs required for build audit")
	}
	library, err := load(assets)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range library.EntriesForAudit() {
		for _, v := range entry.Variants {
			if v.Format != "webp" {
				continue
			}
			t.Run(v.SHA256, func(t *testing.T) {
				t.Parallel()
				original, err := os.ReadFile(filepath.Join(root, v.LegacySHA256+".png"))
				if err != nil {
					t.Fatal(err)
				}
				data, err := library.Read(v.SHA256 + ".webp")
				if err != nil {
					t.Fatal(err)
				}
				h := sha256.Sum256(data)
				if hex.EncodeToString(h[:]) != v.SHA256 {
					t.Fatal("Hash mismatch")
				}
				compareNativePixels(t, original, data)
			})
		}
	}
}

func BenchmarkNativeWebPRead(b *testing.B) {
	library, err := load(assets)
	if err != nil {
		b.Fatal(err)
	}
	var file string
	for _, entry := range library.EntriesForAudit() {
		if entry.Kind == "definition" {
			v := entry.Variants["illustration"]
			file = v.SHA256 + ".webp"
			break
		}
	}
	if file == "" {
		b.Fatal("No illustration")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := library.Read(file); err != nil {
			b.Fatal(err)
		}
	}
}
