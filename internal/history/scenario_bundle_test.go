package history

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
)

func bundlePNG(t *testing.T, red uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: red, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestScenarioBundleRoundTripAndBadMembers(t *testing.T) {
	_, s := worldCampaign(t)
	p := *s.Scenario
	p.ID = "portable-pack"
	p.Presentation = &Presentation{Backdrop: "/history/picture.png"}
	art := bundlePNG(t, 120)
	data, err := EncodeScenarioBundle(p, func(string) ([]byte, error) { return art, nil })
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeScenarioBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	url := decoded.Scenario.Presentation.Backdrop
	if !strings.HasPrefix(url, "/api/history/assets/") || !bytes.Equal(decoded.Assets[url], art) {
		t.Fatal("Image mapping lost")
	}
	repeated, err := EncodeScenarioBundle(decoded.Scenario, func(id string) ([]byte, error) { return decoded.Assets[id], nil })
	if err != nil {
		t.Fatal(err)
	}
	again, err := DecodeScenarioBundle(repeated)
	if err != nil || digest(again.Scenario) != digest(decoded.Scenario) {
		t.Fatal("Portable export changed identity")
	}
	raw, _ := json.Marshal(p)
	for _, name := range []string{"../outside.png", "assets/../outside.png", "C:/outside.png", "config.toml", "assets/unused.png"} {
		var b bytes.Buffer
		z := zip.NewWriter(&b)
		w, _ := z.Create("scenario.json")
		w.Write(raw)
		w, _ = z.Create("assets/picture.png")
		w.Write(art)
		w, _ = z.Create(name)
		w.Write(art)
		z.Close()
		if _, err := DecodeScenarioBundle(b.Bytes()); err == nil {
			t.Fatalf("Accepted unsafe or unused member %q", name)
		}
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.Create("scenario.json")
	w.Write(raw)
	h := &zip.FileHeader{Name: "assets/picture.png"}
	h.SetMode(os.ModeSymlink | 0777)
	w, _ = z.CreateHeader(h)
	w.Write([]byte("outside"))
	z.Close()
	if _, err := DecodeScenarioBundle(b.Bytes()); err == nil {
		t.Fatal("Symlink accepted")
	}
	broken := append([]byte{}, art...)
	broken[len(broken)/2] ^= 255
	if _, err := ScenarioAssetURL(broken, ".png"); err == nil {
		t.Fatal("Corrupt image accepted")
	}
}
