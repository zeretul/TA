package generation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ta/config"
)

// The provider rejects an unknown field in generationConfig outright, so the
// shape of this body decides whether an image is produced at all. This asserts
// on the JSON that leaves the process, not on the struct that built it.
func TestGeminiRequestBodyUsesImageConfig(t *testing.T) {
	png := base64.StdEncoding.EncodeToString(testTinyPNG(t))
	var captured []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Errorf("api key header = %q", got)
		}
		if got := request.URL.Path; got != "/v1/models/gemini-3.1-flash-image:generateContent" {
			t.Errorf("path = %q", got)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		captured = body
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` +
			png + `"}}]}}]}`))
	}))
	defer server.Close()

	adapter := NewGeminiAdapter(server.Client())
	_, err := adapter.Generate(context.Background(), config.ResolvedImageAPIProfile{
		ProfileID: "p", Provider: config.ImageProviderGoogle, Protocol: config.ImageProtocolGemini,
		APIKey: "test-key", BaseURL: server.URL + "/v1", Model: "gemini-3.1-flash-image",
	}, GenerateRequest{Prompt: "画一幅山水", Size: "1376x768", Resolution: "1K", N: 1})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	generation, ok := payload["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("no generationConfig in %s", captured)
	}
	if _, present := generation["responseFormat"]; present {
		t.Fatalf("generationConfig still carries the rejected responseFormat field: %s", captured)
	}
	imageConfig, ok := generation["imageConfig"].(map[string]any)
	if !ok {
		t.Fatalf("generationConfig has no imageConfig: %s", captured)
	}
	if imageConfig["aspectRatio"] != "16:9" || imageConfig["imageSize"] != "1K" {
		t.Fatalf("imageConfig lost the request: %v", imageConfig)
	}
	modalities, ok := generation["responseModalities"].([]any)
	if !ok || len(modalities) != 1 || modalities[0] != "IMAGE" {
		t.Fatalf("responseModalities = %v", generation["responseModalities"])
	}
}

// An unset size and resolution still send an imageConfig object rather than
// omitting it, and an empty one is what the provider accepts: it picks its own
// default framing instead of rejecting the request.
func TestGeminiRequestBodyCarriesImageConfigWhenUnset(t *testing.T) {
	png := base64.StdEncoding.EncodeToString(testTinyPNG(t))
	var captured []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		captured, _ = io.ReadAll(request.Body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` +
			png + `"}}]}}]}`))
	}))
	defer server.Close()

	adapter := NewGeminiAdapter(server.Client())
	if _, err := adapter.Generate(context.Background(), config.ResolvedImageAPIProfile{
		APIKey: "k", BaseURL: server.URL + "/v1", Model: "gemini-3.1-flash-image",
	}, GenerateRequest{Prompt: "画一幅山水", N: 1}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	generation, _ := payload["generationConfig"].(map[string]any)
	imageConfig, ok := generation["imageConfig"].(map[string]any)
	if !ok {
		t.Fatalf("no imageConfig for an unset size: %s", captured)
	}
	for _, rejected := range []string{"aspectRatio", "imageSize"} {
		if _, present := imageConfig[rejected]; present {
			t.Fatalf("unset %s was sent anyway: %s", rejected, captured)
		}
	}
}
