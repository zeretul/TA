package interactive

import "ta/internal/exploration"

// CooperationProjectView exposes only an encountered partner and the player's
// actually discovered instance. It is a read projection, not an authorization
// token. Every draft still goes through the normal formal turn submission.
type CooperationProjectView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	PlayerRole  string `json:"player_role"`
	PartnerRole string `json:"partner_role"`
	// The UI displays the already-known clue using its localized key. Only
	// the source identity and private authored identity stay model-side.
	PartnerSourceKey      string                            `json:"-"`
	ClueText              string                            `json:"clue_text,omitempty"`
	ClueTextKey           string                            `json:"clue_text_key,omitempty"`
	ClueSourceRef         string                            `json:"-"`
	ProjectID             string                            `json:"project_id"`
	ProjectVersion        string                            `json:"project_version"`
	RealmID               string                            `json:"realm_id"`
	InstanceID            string                            `json:"instance_id"`
	NameKey               string                            `json:"name_key"`
	DescriptionKey        string                            `json:"description_key"`
	PlayerRoleKey         string                            `json:"player_role_key"`
	PartnerRoleKey        string                            `json:"partner_role_key"`
	PartnerID             string                            `json:"partner_id"`
	PartnerName           string                            `json:"partner_name"`
	LocationID            string                            `json:"location_id"`
	LocationName          string                            `json:"location_name,omitempty"`
	Phase                 string                            `json:"phase"`
	DurationSeconds       int64                             `json:"duration_seconds"`
	PartnerEnergyCost     int                               `json:"partner_energy_cost"`
	PartnerEnergyLabelKey string                            `json:"partner_energy_label_key,omitempty"`
	PlayerCosts           []exploration.ExploreResourceCost `json:"player_costs"`
	CanAsk                bool                              `json:"can_ask"`
	CanWorkIfAgreed       bool                              `json:"can_work_if_agreed"`
	CanWork               bool                              `json:"can_work"`
	CanReport             bool                              `json:"can_report"`
	BlockedReason         string                            `json:"blocked_reason,omitempty"`
	ReportFacts           []CooperationReportFactView       `json:"report_facts"`
}

// Report facts identify at most four newly shareable outcomes, not an entire
// history or map. The caller must explicitly select what the player shares.
type CooperationReportFactView struct {
	Description    string `json:"description,omitempty"`
	Kind           string `json:"kind"`
	SourceRef      string `json:"source_ref"`
	DescriptionKey string `json:"description_key"`
	ValueKey       string `json:"value_key,omitempty"`
}
