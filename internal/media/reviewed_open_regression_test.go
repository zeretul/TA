package media

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"testing"
)

// This regression deliberately uses the direct byte-serving boundary. Picture
// correctly filters unreviewed entries, but that alone cannot protect Open.
func TestMediaOpenRequiresReviewedBinding(t *testing.T) {
	for _, tc := range []struct {
		name, review string
		allowed      bool
	}{
		{name: "omitted", review: ""},
		{name: "explicit_false", review: `"reviewed":false,`},
		{name: "reviewed", review: `"reviewed":true,`, allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const file = "aaaaaaaaaaaaaaaa.png"
			dir := writeManifest(t, fmt.Sprintf(`{%s"world":"one","source_key":"person","purpose":"portrait","file":%q}`, tc.review, file))
			writeClip(t, dir, file, "image bytes remain protected")
			library := NewLibrary(dir)
			if len(library.Entries()) != 1 {
				t.Fatal("author review inventory must retain the pending entry")
			}
			handle, asset, err := library.Open(file)
			if handle != nil {
				defer handle.Close()
			}
			if !tc.allowed {
				if !errors.Is(err, fs.ErrNotExist) || handle != nil || asset.File != "" {
					t.Fatalf("unreviewed bytes escaped: handle=%v asset=%+v err=%v", handle, asset, err)
				}
				return
			}
			if err != nil || handle == nil || !asset.Reviewed {
				t.Fatalf("reviewed file refused: asset=%+v err=%v", asset, err)
			}
			body, err := io.ReadAll(handle)
			if err != nil || string(body) != "image bytes remain protected" {
				t.Fatalf("reviewed bytes changed: %q err=%v", body, err)
			}
		})
	}
}
