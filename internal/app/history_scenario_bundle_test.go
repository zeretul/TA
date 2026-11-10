package app

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"path"
	"ta/internal/history"
	"testing"
)

func TestHistoryBundlePortableAndDoesNotOverwriteArtwork(t *testing.T) {
	service, projectID, _ := historyServiceFixture(t, func(historyModelRequest) (string, int) {
		t.Fatal("Bundle import must not call a model")
		return "", 500
	})
	makeBundle := func(id string, red uint8) []byte {
		t.Helper()
		p, err := service.ExportScenario(projectID, "ming-1627")
		if err != nil {
			t.Fatal(err)
		}
		p.ID = id
		p.Presentation = &history.Presentation{Backdrop: "/history/same-name.png"}
		img := image.NewRGBA(image.Rect(0, 0, 2, 2))
		img.Set(0, 0, color.RGBA{R: red, A: 255})
		var art bytes.Buffer
		png.Encode(&art, img)
		b, err := history.EncodeScenarioBundle(p, func(string) ([]byte, error) { return art.Bytes(), nil })
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	first, err := service.ImportScenarioPackage(context.Background(), projectID, makeBundle("portable-one", 20))
	if err != nil {
		t.Fatal(err)
	}
	same, err := service.ImportScenarioPackage(context.Background(), projectID, makeBundle("portable-one", 20))
	if err != nil || same.ID != first.ID {
		t.Fatal("Duplicate import failed", err)
	}
	old, _, err := service.ScenarioAsset(path.Base(first.Presentation.Backdrop))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ImportScenarioPackage(context.Background(), projectID, makeBundle("portable-one", 80)); err == nil {
		t.Fatal("Different content overwrote existing ID")
	}
	second, err := service.ImportScenarioPackage(context.Background(), projectID, makeBundle("portable-two", 80))
	if err != nil || second.Presentation.Backdrop == first.Presentation.Backdrop {
		t.Fatal("Same filename images collided", err)
	}
	after, _, _ := service.ScenarioAsset(path.Base(first.Presentation.Backdrop))
	if !bytes.Equal(old, after) {
		t.Fatal("Existing image changed")
	}
	exported, err := service.ExportScenarioBundle(projectID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, otherID, _ := historyServiceFixture(t, func(historyModelRequest) (string, int) { return "", 500 })
	imported, err := other.ImportScenarioPackage(context.Background(), otherID, exported)
	if err != nil {
		t.Fatal(err)
	}
	imageBytes, _, err := other.ScenarioAsset(path.Base(imported.Presentation.Backdrop))
	if err != nil || !bytes.Equal(imageBytes, old) {
		t.Fatal("Second data root lost image", err)
	}
	if _, err := service.ImportScenarioPackage(context.Background(), projectID, []byte("PKbroken")); err == nil {
		t.Fatal("Broken archive accepted")
	}
}
