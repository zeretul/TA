package tools

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"ta/config"
	"ta/internal/book"
	imagegen "ta/internal/image/generation"
	"ta/internal/media"
	"ta/internal/portrait"
)

// referenceLabelMaxRunes keeps one person's label from crowding out the others.
// The label is read next to the picture it names, so it only has to say who this
// face is and what the story already said about it.
const referenceLabelMaxRunes = 400

// interactiveImageReferences turns the cast carried by this run into request
// parts for the image model.
//
// Nothing here is model-visible: the cast is not in the tool schema, so the
// model cannot ask for a face the product did not choose. A person whose picture
// cannot be read still contributes their appearance as text, because a described
// face is better than a wrong one, and an image with no references at all is
// still an image.
func interactiveImageReferences(cfg *config.Config, bookService *book.Service) []imagegen.ReferenceImage {
	if cfg == nil {
		return nil
	}
	people := imagegen.DecodeReferencePeople(cfg.ImageReferencePeople)
	if len(people) == 0 {
		return nil
	}
	workspace := strings.TrimSpace(cfg.Workspace)
	shipped := portrait.NewLibrary()
	var library *media.Library
	mediaLoaded := false

	out := make([]imagegen.ReferenceImage, 0, len(people))
	for _, person := range people {
		label := referenceLabel(person)
		source, name := imagegen.SplitReferencePicture(person.Picture)
		data, mimeType := readReferencePicture(source, name, workspace, bookService, shipped, func() *media.Library {
			if !mediaLoaded {
				library = media.NewLibrary(cfg.MediaDir)
				mediaLoaded = true
			}
			return library
		})
		if len(data) == 0 {
			continue
		}
		out = append(out, imagegen.ReferenceImage{Label: label, Data: data, MIMEType: mimeType})
	}
	return out
}

// referenceLabel says who the attached picture is and how the story described
// them. The picture and its label have to stay adjacent in the request for the
// model to bind a name to a face.
func referenceLabel(person imagegen.ReferencePerson) string {
	name := strings.TrimSpace(person.Name)
	if appearance := strings.TrimSpace(person.Appearance); appearance != "" {
		return name + " — " + limitReferenceRunes(appearance, referenceLabelMaxRunes)
	}
	return name
}

func limitReferenceRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if limit <= 0 || len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func readReferencePicture(
	source, name, workspace string,
	bookService *book.Service,
	shipped *portrait.Library,
	mediaLibrary func() *media.Library,
) ([]byte, string) {
	switch source {
	case imagegen.PictureInMedia:
		library := mediaLibrary()
		if library == nil {
			return nil, ""
		}
		handle, asset, err := library.Open(name)
		if err != nil {
			slog.WarnContext(context.Background(), fmt.Sprintf("[image-tool] open media reference failed file=%s err=%v", name, err))
			return nil, ""
		}
		defer handle.Close()
		data, err := readBounded(handle)
		if err != nil {
			return nil, ""
		}
		return data, media.ContentType(asset.File)
	case imagegen.PictureShipped:
		entry, ok := shipped.Lookup(name)
		if !ok {
			return nil, ""
		}
		data, mimeType, err := shipped.Read(entry.File, false)
		if err != nil {
			slog.WarnContext(context.Background(), fmt.Sprintf("[image-tool] read shipped portrait failed key=%s err=%v", name, err))
			return nil, ""
		}
		return data, mimeType
	default:
		if bookService == nil {
			return nil, ""
		}
		absolute, err := book.SafePath(workspace, name)
		if err != nil {
			return nil, ""
		}
		handle, err := os.Open(absolute)
		if err != nil {
			return nil, ""
		}
		defer handle.Close()
		data, err := readBounded(handle)
		if err != nil {
			return nil, ""
		}
		return data, referenceMIMEType(name)
	}
}

// mediaReferenceMaxBytes bounds one reference. A face is a small picture; a file
// larger than this is a mistake, and sending it would spend the request's budget
// on one frame.
const mediaReferenceMaxBytes = 8 << 20

func readBounded(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, mediaReferenceMaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > mediaReferenceMaxBytes {
		return nil, fs.ErrInvalid
	}
	return data, nil
}

// referenceMIMEType names the picture's own format. A workspace file is not in
// the media manifest, so its type comes from its extension rather than a lookup.
func referenceMIMEType(name string) string {
	mimeType := media.ContentType(name)
	if strings.HasPrefix(mimeType, "video/") {
		return ""
	}
	return mimeType
}
