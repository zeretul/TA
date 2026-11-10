package catalogart

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"strings"

	"golang.org/x/image/webp"
)

type encodedAsset struct {
	format string
}

func validateEncodedVariant(source fs.FS, v Variant) (encodedAsset, error) {
	format := v.Format
	if format == "" {
		format = "png"
	}
	if format != "png" && format != "webp" {
		return encodedAsset{}, fmt.Errorf("unsupported artwork format")
	}
	if v.LegacySHA256 != "" && (format != "webp" || !validHash(v.LegacySHA256) || v.LegacySHA256 == v.SHA256) {
		return encodedAsset{}, fmt.Errorf("invalid legacy artwork identity")
	}
	data, err := fs.ReadFile(source, "assets/images/"+v.SHA256+"."+format)
	if err != nil || len(data) > 8*1024*1024 {
		return encodedAsset{}, fmt.Errorf("missing or oversized artwork")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != v.SHA256 {
		return encodedAsset{}, fmt.Errorf("artwork checksum mismatch")
	}
	var config image.Config
	if format == "png" {
		config, err = png.DecodeConfig(bytes.NewReader(data))
	} else {
		if len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:16]) != "WEBPVP8L" {
			return encodedAsset{}, fmt.Errorf("artwork is not plain lossless WebP")
		}
		config, err = webp.DecodeConfig(bytes.NewReader(data))
	}
	if err != nil || config.Width != v.Width || config.Height != v.Height {
		return encodedAsset{}, fmt.Errorf("invalid artwork dimensions")
	}
	return encodedAsset{format: format}, nil
}

// CanonicalFile accepts current image filenames, the frontend's existing
// <current-hash>.png spelling, and verified previous PNG or WebP identities. Legacy
// spellings must redirect to the canonical URL before any bytes are served.
func (l *Library) CanonicalFile(file string) string {
	if l == nil {
		return ""
	}
	id, extension, found := strings.Cut(file, ".")
	if !found || !validHash(id) || (extension != "png" && extension != "webp") {
		return ""
	}
	if asset, ok := l.files[id]; ok {
		if extension == asset.format || (extension == "png" && asset.format == "webp") {
			return id + "." + asset.format
		}
		return ""
	}
	if canonical := l.aliases[id]; canonical != "" {
		return canonical + "." + l.files[canonical].format
	}
	return ""
}

func (l *Library) ContentType(file string) string {
	if l.CanonicalFile(file) != file || file == "" {
		return ""
	}
	if strings.HasSuffix(file, ".webp") {
		return "image/webp"
	}
	return "image/png"
}
