package generation

import (
	"context"

	"ta/config"
)

type GenerateRequest struct {
	ProfileID    string `json:"profile_id,omitempty"`
	Prompt       string `json:"prompt"`
	N            int    `json:"n,omitempty"`
	Size         string `json:"size,omitempty"`
	AspectRatio  string `json:"aspect_ratio,omitempty"`
	Resolution   string `json:"resolution,omitempty"`
	Quality      string `json:"quality,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
	// ReferenceImages are pictures the provider should look at while drawing.
	// They travel as request parts, never as URLs, so the provider never learns a
	// local path. Only protocols with a multimodal request body consume them; a
	// text-only protocol ignores the field rather than failing, because a story
	// must still produce an image when its provider cannot take references.
	ReferenceImages []ReferenceImage `json:"reference_images,omitempty"`
}

// ReferenceImage is one already-authorized picture supplied to the image model.
// Label names what it shows so the model can tell two faces apart.
type ReferenceImage struct {
	Label    string
	Data     []byte
	MIMEType string
}

type Result struct {
	ProfileID    string
	Provider     string
	Model        string
	Created      int64
	Size         string
	Quality      string
	OutputFormat string
	Images       []Image
	Failures     []Failure
}

// Failure reports an item-level provider failure while preserving successful
// images from the same logical request.
type Failure struct {
	Index   int    `json:"index"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Image struct {
	Data          []byte
	MIMEType      string
	Extension     string
	RevisedPrompt string
	SourceURL     string
}

type Adapter interface {
	Generate(ctx context.Context, profile config.ResolvedImageAPIProfile, request GenerateRequest) (Result, error)
}
