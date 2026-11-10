package storypack

import (
	"fmt"
	"strings"
)

func creationHasID(ids []string, wanted string) bool {
	for _, id := range ids {
		if strings.TrimSpace(id) == wanted {
			return true
		}
	}
	return false
}

// These declarations constrain only existing creation choices. They do not
// evaluate free-form biographies, grant mechanics or remove authored templates.
func (p Pack) validateCreationRestrictions() error {
	c := p.Creation
	if c == nil {
		return nil
	}
	if c.CustomLocationPolicy != "" && c.CustomLocationPolicy != "authored_only" {
		return fmt.Errorf("unsupported custom starting-location policy: %s", c.CustomLocationPolicy)
	}
	seen := map[string]bool{}
	for _, id := range c.VisibleOriginIDs {
		if _, ok := c.Origin(id); !ok || seen[id] {
			return fmt.Errorf("unknown or duplicate visible creation origin: %s", id)
		}
		seen[id] = true
	}
	seen = map[string]bool{}
	for _, id := range c.CustomStateBasisTemplates {
		if !p.isStateBasisTemplate(id) || seen[id] {
			return fmt.Errorf("unknown or duplicate custom state basis: %s", id)
		}
		seen[id] = true
	}
	if custom, ok := c.Origin("custom"); ok && len(c.CustomStateBasisTemplates) > 0 {
		basis := strings.TrimSpace(custom.TemplateID)
		if basis == "" {
			basis = strings.TrimSpace(custom.DefaultTemplateID)
		}
		if !creationHasID(c.CustomStateBasisTemplates, basis) {
			return fmt.Errorf("custom origin default is not an allowed custom state basis: %s", basis)
		}
	}
	for _, origin := range c.Origins {
		if origin.DefaultLocationID == "" {
			continue
		}
		if _, ok := c.Location(origin.DefaultLocationID); !ok || origin.DefaultLocationID == "auto" {
			return fmt.Errorf("creation origin has no concrete authored default location: %s", origin.ID)
		}
	}
	return nil
}
