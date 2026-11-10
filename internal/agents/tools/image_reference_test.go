package tools

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ta/config"
	"ta/internal/book"
	imagegen "ta/internal/image/generation"
	"ta/internal/media"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewNRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buffer.Bytes()
}

// A run that carries no cast asks for no references. This is the state every
// story that has not bound a picture to anyone is in, and it has to stay a
// no-op rather than an error.
func TestInteractiveImageReferencesWithoutCastIsEmpty(t *testing.T) {
	if got := interactiveImageReferences(&config.Config{}, nil); len(got) != 0 {
		t.Fatalf("expected no references, got %d", len(got))
	}
	if got := interactiveImageReferences(nil, nil); len(got) != 0 {
		t.Fatalf("nil config produced %d references", len(got))
	}
}

// The shipped portrait is the fallback face: a person of the world pack with no
// bound picture still travels as the picture the player is shown.
func TestInteractiveImageReferencesReadsShippedPortrait(t *testing.T) {
	cast := imagegen.EncodeReferencePeople([]imagegen.ReferencePerson{{
		Name:       "陈巧倩",
		SourceKey:  "canon_chen_qiaoqian",
		Appearance: "五官清秀，衣饰整洁。",
		Picture:    imagegen.ReferencePicture(imagegen.PictureShipped, "canon_chen_qiaoqian"),
	}})
	got := interactiveImageReferences(&config.Config{ImageReferencePeople: cast}, nil)
	if len(got) != 1 {
		t.Fatalf("expected one reference, got %d", len(got))
	}
	if !strings.Contains(got[0].Label, "陈巧倩") || !strings.Contains(got[0].Label, "五官清秀") {
		t.Fatalf("label lost the name or the appearance: %q", got[0].Label)
	}
	if len(got[0].Data) == 0 || got[0].MIMEType != "image/png" {
		t.Fatalf("shipped portrait not read: %d bytes, %q", len(got[0].Data), got[0].MIMEType)
	}
}

// An unknown author key contributes no picture. A wrong face is worse than a
// described one, so the person is dropped rather than given someone else's.
func TestInteractiveImageReferencesDropsUnknownShippedPortrait(t *testing.T) {
	cast := imagegen.EncodeReferencePeople([]imagegen.ReferencePerson{{
		Name:      "无名氏",
		SourceKey: "canon_nobody_at_all",
		Picture:   imagegen.ReferencePicture(imagegen.PictureShipped, "canon_nobody_at_all"),
	}})
	if got := interactiveImageReferences(&config.Config{ImageReferencePeople: cast}, nil); len(got) != 0 {
		t.Fatalf("unknown key produced %d references", len(got))
	}
}

// A picture the story bound to the profile lives in the workspace, which is the
// first place the character window looks.
func TestInteractiveImageReferencesReadsWorkspacePicture(t *testing.T) {
	workspace := t.TempDir()
	relative := filepath.ToSlash(filepath.Join("assets", "avatars", "hero.png"))
	absolute := filepath.Join(workspace, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(absolute, testPNG(t), 0o644); err != nil {
		t.Fatalf("write png: %v", err)
	}

	cast := imagegen.EncodeReferencePeople([]imagegen.ReferencePerson{{
		Name:    "韩掌柜",
		Picture: imagegen.ReferencePicture(imagegen.PictureInWorkspace, relative),
	}})
	got := interactiveImageReferences(
		&config.Config{Workspace: workspace, ImageReferencePeople: cast},
		book.NewService(workspace),
	)
	if len(got) != 1 {
		t.Fatalf("expected one reference, got %d", len(got))
	}
	if got[0].MIMEType != "image/png" || len(got[0].Data) == 0 {
		t.Fatalf("workspace picture not read: %d bytes, %q", len(got[0].Data), got[0].MIMEType)
	}
}

// The same locator with no book service resolves nothing: the workspace reader
// is the book service, and there is no other way to reach the file.
func TestInteractiveImageReferencesWorkspaceNeedsBookService(t *testing.T) {
	workspace := t.TempDir()
	relative := filepath.ToSlash(filepath.Join("assets", "avatars", "hero.png"))
	absolute := filepath.Join(workspace, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(absolute, testPNG(t), 0o644); err != nil {
		t.Fatalf("write png: %v", err)
	}
	cast := imagegen.EncodeReferencePeople([]imagegen.ReferencePerson{{
		Name:    "韩掌柜",
		Picture: imagegen.ReferencePicture(imagegen.PictureInWorkspace, relative),
	}})
	if got := interactiveImageReferences(&config.Config{Workspace: workspace, ImageReferencePeople: cast}, nil); len(got) != 0 {
		t.Fatalf("workspace picture resolved without a book service: %d references", len(got))
	}
}

// A picture imported beside the binary is what the player already sees when the
// story has replaced a face, so it outranks the shipped portrait.
func TestInteractiveImageReferencesReadsMediaPicture(t *testing.T) {
	mediaDir := t.TempDir()
	library := media.NewLibrary(mediaDir)
	asset, err := library.Import(media.ImportRequest{
		World:    "fanren-jie",
		SourceKey: "canon_chen_qiaoqian",
		Purpose:  media.PurposeAvatar,
		Role:     media.RoleGeneral,
		Filename: "face.png",
		Data:     testPNG(t),
	})
	if err != nil {
		t.Fatalf("import media: %v", err)
	}

	cast := imagegen.EncodeReferencePeople([]imagegen.ReferencePerson{{
		Name:    "陈巧倩",
		Picture: imagegen.ReferencePicture(imagegen.PictureInMedia, asset.File),
	}})
	got := interactiveImageReferences(&config.Config{MediaDir: mediaDir, ImageReferencePeople: cast}, nil)
	if len(got) != 1 {
		t.Fatalf("expected one reference, got %d", len(got))
	}
	if got[0].MIMEType != "image/png" || len(got[0].Data) == 0 {
		t.Fatalf("media picture not read: %d bytes, %q", len(got[0].Data), got[0].MIMEType)
	}
}

// A locator that climbs out of the workspace is dropped. The cast is built from
// stored state, but the state is user-writable, so the reader refuses rather
// than trusts it.
func TestInteractiveImageReferencesRefusesEscapingPath(t *testing.T) {
	workspace := t.TempDir()
	secret := filepath.Join(filepath.Dir(workspace), "secret.png")
	if err := os.WriteFile(secret, testPNG(t), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	cast := imagegen.EncodeReferencePeople([]imagegen.ReferencePerson{{
		Name:    "逃逸",
		Picture: imagegen.ReferencePicture(imagegen.PictureInWorkspace, "../secret.png"),
	}})
	if got := interactiveImageReferences(
		&config.Config{Workspace: workspace, ImageReferencePeople: cast},
		book.NewService(workspace),
	); len(got) != 0 {
		t.Fatalf("escaping path produced %d references", len(got))
	}
}
