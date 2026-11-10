package cultivation

import (
	"fmt"
	"path"
	"strings"
	"ta/internal/catalogart"
)

// DefinitionArt is presentation-only. A generated asset belongs to an explicit
// Project; neither its pixels nor its metadata can authorize a game capability.
type DefinitionArt struct {
	Source    string `json:"source"`
	CatalogID string `json:"catalog_id,omitempty"`
	PresetID  string `json:"preset_id,omitempty"`
	ImagePath string `json:"image_path,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

func (a DefinitionArt) Validate() error {
	switch a.Source {
	case "catalog":
		if a.PresetID == "" && a.ProjectID == "" && a.ImagePath == "" && catalogart.Default().IsDefinitionAsset(a.CatalogID) {
			return nil
		}
	case "preset":
		if a.ImagePath != "" || a.ProjectID != "" || a.CatalogID != "" {
			return fmt.Errorf("preset art cannot address project assets")
		}
		for _, theme := range []string{"cultivation", "wuxia", "space"} {
			for _, category := range []string{"technique", "skill", "equipment"} {
				if a.PresetID == theme+"-"+category {
					return nil
				}
			}
		}
	case "generated":
		if a.CatalogID == "" && a.PresetID == "" && a.ProjectID != "" && len(a.ProjectID) <= 200 && SafeDefinitionImagePath(a.ImagePath) {
			return nil
		}
	}
	return fmt.Errorf("invalid definition art reference")
}

// SafeDefinitionImagePath admits only the existing generated-image asset root,
// never URLs, host paths, traversal, metadata files or executable SVG payloads.
func SafeDefinitionImagePath(value string) bool {
	if value == "" || strings.ContainsAny(value, "\\\x00:?#<>\"|*") || path.Clean(value) != value || !strings.HasPrefix(value, "assets/image/generated/") {
		return false
	}
	for _, r := range value {
		if r < 32 {
			return false
		}
	}
	switch strings.ToLower(path.Ext(value)) {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	}
	return false
}
