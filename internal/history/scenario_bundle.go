package history

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	_ "golang.org/x/image/webp"
)

const MaxScenarioBundleBytes = 64 << 20
const MaxScenarioAssetBytes = 16 << 20
const maxScenarioExpandedBytes = 128 << 20

// ScenarioBundle contains validated author data and referenced artwork only.
// Assets are keyed by canonical content-addressed URL, never by a host path.
type ScenarioBundle struct {
	Scenario Scenario
	Assets   map[string][]byte
}

func ScenarioAssetURLs(s Scenario) []string {
	seen := map[string]bool{}
	if s.Presentation != nil {
		if s.Presentation.Backdrop != "" {
			seen[s.Presentation.Backdrop] = true
		}
		for _, p := range s.Presentation.Portraits {
			seen[p.Asset] = true
		}
	}
	result := make([]string, 0, len(seen))
	for url := range seen {
		result = append(result, url)
	}
	sort.Strings(result)
	return result
}

func ScenarioAssetURL(data []byte, ext string) (string, error) {
	if len(data) == 0 || len(data) > MaxScenarioAssetBytes {
		return "", invalid("scenario image size exceeds limit")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || ("."+format) != ext || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 32*1024*1024 {
		return "", invalid("invalid scenario image")
	}
	if _, decodedFormat, decodeErr := image.Decode(bytes.NewReader(data)); decodeErr != nil || decodedFormat != format {
		return "", invalid("corrupt scenario image")
	}
	hash := sha256.Sum256(data)
	return "/api/history/assets/" + hex.EncodeToString(hash[:]) + ext, nil
}

func ReplaceScenarioAssets(s Scenario, replacements map[string]string) Scenario {
	if s.Presentation == nil {
		return s
	}
	p := *s.Presentation
	p.Portraits = map[string]Portrait{}
	if value, ok := replacements[p.Backdrop]; ok {
		p.Backdrop = value
	}
	for id, portrait := range s.Presentation.Portraits {
		if value, ok := replacements[portrait.Asset]; ok {
			portrait.Asset = value
		}
		p.Portraits[id] = portrait
	}
	s.Presentation = &p
	return s
}

// DecodeScenarioBundle validates every member before any application write.
// ZIP directories are optional; traversal, links, duplicate names, unused files,
// unsupported images and incomplete references are rejected.
func DecodeScenarioBundle(data []byte) (ScenarioBundle, error) {
	result := ScenarioBundle{Assets: map[string][]byte{}}
	if len(data) > MaxScenarioBundleBytes {
		return result, invalid("scenario bundle exceeds 64 MiB")
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return result, invalid("invalid scenario ZIP")
	}
	if len(z.File) > 400 {
		return result, invalid("too many scenario bundle entries")
	}
	members := map[string]*zip.File{}
	seen := map[string]bool{}
	var total uint64
	for _, f := range z.File {
		name := f.Name
		if name == "assets/" && f.FileInfo().IsDir() && f.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if !f.Mode().IsRegular() || strings.Contains(name, "\\") || strings.Contains(name, ":") || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || seen[strings.ToLower(name)] {
			return result, invalid("unsafe or duplicate scenario member")
		}
		if name != "scenario.json" && (!strings.HasPrefix(name, "assets/") || strings.Count(name, "/") != 1 || !historyAssetPath.MatchString("/history/"+path.Base(name))) {
			return result, invalid("unexpected scenario member")
		}
		total += f.UncompressedSize64
		if f.UncompressedSize64 > MaxScenarioAssetBytes || total > maxScenarioExpandedBytes {
			return result, invalid("expanded scenario bundle too large")
		}
		seen[strings.ToLower(name)] = true
		members[name] = f
	}
	read := func(name string, max int) ([]byte, error) {
		f := members[name]
		if f == nil {
			return nil, invalid("scenario bundle member missing: %s", name)
		}
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer r.Close()
		b, err := io.ReadAll(io.LimitReader(r, int64(max)+1))
		if err != nil || len(b) > max {
			return nil, invalid("scenario member unreadable or too large")
		}
		return b, nil
	}
	raw, err := read("scenario.json", MaxScenarioBytes)
	if err != nil {
		return result, err
	}
	s, err := DecodeScenario(raw)
	if err != nil {
		return result, err
	}
	used := map[string]bool{"scenario.json": true}
	replacements := map[string]string{}
	for _, url := range ScenarioAssetURLs(s) {
		name := "assets/" + path.Base(url)
		b, err := read(name, MaxScenarioAssetBytes)
		if err != nil {
			return result, err
		}
		canonical, err := ScenarioAssetURL(b, path.Ext(url))
		if err != nil {
			return result, err
		}
		if strings.HasPrefix(url, "/api/history/assets/") && url != canonical {
			return result, invalid("scenario asset digest mismatch")
		}
		replacements[url] = canonical
		result.Assets[canonical] = b
		used[name] = true
	}
	for name := range members {
		if !used[name] {
			return result, invalid("unreferenced bundle member: %s", name)
		}
	}
	result.Scenario = ReplaceScenarioAssets(s, replacements)
	canonicalJSON, err := json.Marshal(result.Scenario)
	if err != nil || len(canonicalJSON) > MaxScenarioBytes {
		return result, invalid("canonical scenario JSON exceeds 2 MiB")
	}
	return result, ValidateScenario(result.Scenario)
}

// EncodeScenarioBundle exports exactly the selected scenario's images. The
// reader must resolve the app's real static root or content-addressed storage.
func EncodeScenarioBundle(s Scenario, read func(string) ([]byte, error)) ([]byte, error) {
	if err := ValidateScenario(s); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	z := zip.NewWriter(&buffer)
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	write := func(name string, data []byte) error {
		w, err := z.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if len(raw) > MaxScenarioBytes {
		return nil, invalid("scenario JSON exceeds 2 MiB")
	}
	if err = write("scenario.json", raw); err != nil {
		return nil, err
	}
	names := map[string][]byte{}
	total := len(raw)
	for _, url := range ScenarioAssetURLs(s) {
		b, err := read(url)
		if err != nil {
			return nil, fmt.Errorf("scenario artwork %s: %w", url, err)
		}
		canonical, err := ScenarioAssetURL(b, path.Ext(url))
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(url, "/api/history/assets/") && canonical != url {
			return nil, invalid("scenario asset digest mismatch")
		}
		name := "assets/" + path.Base(url)
		if old, ok := names[name]; ok {
			if !bytes.Equal(old, b) {
				return nil, invalid("conflicting scenario image names")
			}
			continue
		}
		names[name] = b
		total += len(b)
		if total > maxScenarioExpandedBytes {
			return nil, invalid("scenario artwork exceeds limit")
		}
		if err = write(name, b); err != nil {
			return nil, err
		}
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	if buffer.Len() > MaxScenarioBundleBytes {
		return nil, invalid("scenario ZIP exceeds limit")
	}
	return buffer.Bytes(), nil
}
