package generation

import (
	"encoding/base64"
	"testing"
)

// The request the image model actually receives is what proves a reference
// travelled. This asserts on the parts, not on an intermediate struct.
func TestGeminiImagePartsCarryEachReferenceBesideItsLabel(t *testing.T) {
	request := GenerateRequest{
		Prompt: "画一幅山水",
		ReferenceImages: []ReferenceImage{
			{Label: "陈巧倩 — 五官清秀。", Data: []byte{1, 2, 3}, MIMEType: "image/png"},
			{Label: "韩掌柜 — 圆脸。", Data: []byte{4, 5}, MIMEType: "image/jpeg"},
		},
	}
	parts := geminiImageParts(request)

	// prompt, then a label and a picture for each of the two people.
	if len(parts) != 5 {
		t.Fatalf("expected 5 parts, got %d: %#v", len(parts), parts)
	}
	if parts[0].Text != request.Prompt || parts[0].InlineData != nil {
		t.Fatalf("prompt is not first: %#v", parts[0])
	}
	for index, reference := range request.ReferenceImages {
		labelPart := parts[1+index*2]
		picturePart := parts[2+index*2]
		if labelPart.Text != reference.Label || labelPart.InlineData != nil {
			t.Fatalf("label %d missing or misplaced: %#v", index, labelPart)
		}
		if picturePart.InlineData == nil {
			t.Fatalf("picture %d has no inline data: %#v", index, picturePart)
		}
		if picturePart.InlineData.MIMEType != reference.MIMEType {
			t.Fatalf("picture %d type is %q, want %q", index, picturePart.InlineData.MIMEType, reference.MIMEType)
		}
		if picturePart.InlineData.Data != base64.StdEncoding.EncodeToString(reference.Data) {
			t.Fatalf("picture %d bytes changed in transit", index)
		}
	}
}

// A reference with no bytes is dropped rather than sent as an empty inline
// part, which the provider would reject.
func TestGeminiImagePartsSkipEmptyReference(t *testing.T) {
	parts := geminiImageParts(GenerateRequest{
		Prompt:          "画一幅山水",
		ReferenceImages: []ReferenceImage{{Label: "空", Data: nil, MIMEType: "image/png"}},
	})
	if len(parts) != 1 {
		t.Fatalf("expected only the prompt, got %d parts", len(parts))
	}
}

// An image with no references at all is still an image: the prompt alone is a
// valid request.
func TestGeminiImagePartsWithoutReferencesIsJustThePrompt(t *testing.T) {
	parts := geminiImageParts(GenerateRequest{Prompt: "画一幅山水"})
	if len(parts) != 1 || parts[0].Text != "画一幅山水" {
		t.Fatalf("unexpected parts: %#v", parts)
	}
}
