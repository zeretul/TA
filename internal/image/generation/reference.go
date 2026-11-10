package generation

import (
	"encoding/json"
	"strings"
)

// ReferencePerson is one person the story has already put in a scene, together
// with what the product knows about how they look. It is what the image tool
// turns into request parts: the appearance goes to the model as text, and the
// picture goes as an inline part so the model draws the face the story already
// has instead of inventing one.
//
// It travels as a JSON string through config because the config package must not
// learn about portraits, pictures or request parts. That is the same route the
// per-run image preset prompt already takes.
type ReferencePerson struct {
	// Name is the name the story uses for this person.
	Name string `json:"name"`
	// SourceKey is the author's stable identity, empty for a story-invented
	// person. It is what selects a shipped default portrait.
	SourceKey string `json:"source_key,omitempty"`
	// Appearance is the author's own description of how they look, as the world
	// book records it. It is empty when the story never established one.
	Appearance string `json:"appearance,omitempty"`
	// Picture locates the one picture this person is shown with, as produced by
	// ReferencePicture. It outranks the shipped portrait because it is the
	// picture the player already sees. Empty means no picture was found, and the
	// appearance alone has to carry the face.
	Picture string `json:"picture,omitempty"`
}

// Reference picture locators. They name where a picture lives rather than carry
// it: the cast travels inside a run record that is rebuilt from JSON, and a
// base64 blob does not belong in that record. Whoever holds the same workspace,
// media directory and shipped portraits resolves the locator to bytes.
const (
	PictureInWorkspace = "workspace:"
	PictureInMedia     = "media:"
	PictureShipped     = "portrait:"
)

// ReferencePicture builds a locator for one place a picture can live.
func ReferencePicture(source, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return source + name
}

// SplitReferencePicture takes a locator apart. A value with no known prefix is
// treated as a workspace path, which is what a locator written before this
// existed would have been.
func SplitReferencePicture(locator string) (string, string) {
	locator = strings.TrimSpace(locator)
	switch {
	case strings.HasPrefix(locator, PictureInWorkspace):
		return PictureInWorkspace, strings.TrimPrefix(locator, PictureInWorkspace)
	case strings.HasPrefix(locator, PictureInMedia):
		return PictureInMedia, strings.TrimPrefix(locator, PictureInMedia)
	case strings.HasPrefix(locator, PictureShipped):
		return PictureShipped, strings.TrimPrefix(locator, PictureShipped)
	}
	return PictureInWorkspace, locator
}

// EncodeReferencePeople serialises the cast for one run. An empty cast encodes
// as the empty string so a caller can tell "no cast" from "a cast of none".
func EncodeReferencePeople(people []ReferencePerson) string {
	if len(people) == 0 {
		return ""
	}
	encoded, err := json.Marshal(people)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// DecodeReferencePeople reads a cast back. A malformed value yields no cast
// rather than an error: an image that cannot reuse a face is still an image, and
// failing the whole run over a decoration would lose the turn's picture.
func DecodeReferencePeople(raw string) []ReferencePerson {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var people []ReferencePerson
	if err := json.Unmarshal([]byte(raw), &people); err != nil {
		return nil
	}
	out := make([]ReferencePerson, 0, len(people))
	for _, person := range people {
		person.Name = strings.TrimSpace(person.Name)
		person.SourceKey = strings.TrimSpace(person.SourceKey)
		person.Appearance = strings.TrimSpace(person.Appearance)
		person.Picture = strings.TrimSpace(person.Picture)
		if person.Name == "" {
			continue
		}
		out = append(out, person)
	}
	return out
}
