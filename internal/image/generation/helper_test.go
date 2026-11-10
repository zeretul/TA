package generation

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func testTinyPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewNRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buffer.Bytes()
}
