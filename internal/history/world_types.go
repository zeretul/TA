package history

// Text is authored display text. Runtime prose follows the campaign language.
type Text struct {
	ZH string `json:"zh"`
	EN string `json:"en"`
}

type MetricDefinition struct {
	ID   string `json:"id"`
	Name Text   `json:"name"`
	Unit Text   `json:"unit"`
	Min  int64  `json:"min"`
	Max  int64  `json:"max"`
}

type ResourceDefinition struct {
	ID             string `json:"id"`
	Name           Text   `json:"name"`
	Unit           Text   `json:"unit"`
	MaxTransaction int64  `json:"max_transaction"`
}

// Audience contains public, player, or exact minister IDs. An empty audience
// is simulator-only truth; it must never be projected into a player response.
type Entity struct {
	ID         string           `json:"id"`
	Kind       string           `json:"kind"`
	Name       Text             `json:"name"`
	Status     string           `json:"status"`
	Location   string           `json:"location,omitempty"`
	Owner      string           `json:"owner,omitempty"`
	Attributes map[string]Text  `json:"attributes"`
	Metrics    map[string]int64 `json:"metrics"`
	Audience   []string         `json:"audience"`
}

type Account struct {
	ID       string   `json:"id"`
	Name     Text     `json:"name"`
	Resource string   `json:"resource"`
	Owner    string   `json:"owner"`
	Balance  int64    `json:"balance"`
	Audience []string `json:"audience"`
}

type WorldEvent struct {
	TitleEN    string   `json:"title_en,omitempty"`
	TextEN     string   `json:"text_en,omitempty"`
	FollowUpEN string   `json:"follow_up_en,omitempty"`
	ID         string   `json:"id"`
	Turn       int      `json:"turn"`
	Title      string   `json:"title"`
	Text       string   `json:"text"`
	EntityIDs  []string `json:"entity_ids"`
	OrderIDs   []string `json:"order_ids"`
	Audience   []string `json:"audience"`
	FollowUp   string   `json:"follow_up,omitempty"`
	Status     string   `json:"status"`
}

type Objective struct {
	ID       string `json:"id"`
	Name     Text   `json:"name"`
	EntityID string `json:"entity_id"`
	Metric   string `json:"metric"`
	Operator string `json:"operator"`
	Value    int64  `json:"value"`
	Ending   string `json:"ending"`
}

type Calendar struct {
	Name      Text `json:"name"`
	Year      int  `json:"year"`
	Quarter   int  `json:"quarter"`
	EraOffset int  `json:"era_offset"`
}

// Scenario is a self-contained data package, never executable code or a path.
// The full validated package and digest are frozen in each new campaign.
type Scenario struct {
	PlayerEntityID string               `json:"player_entity_id,omitempty"`
	Presentation   *Presentation        `json:"presentation,omitempty"`
	Schema         int                  `json:"schema"`
	ID             string               `json:"id"`
	Version        string               `json:"version"`
	Name           Text                 `json:"name"`
	Description    Text                 `json:"description"`
	Calendar       Calendar             `json:"calendar"`
	Rules          string               `json:"rules"`
	Provenance     string               `json:"provenance"`
	Metrics        []MetricDefinition   `json:"metrics"`
	Resources      []ResourceDefinition `json:"resources"`
	Ministers      []Minister           `json:"ministers"`
	Initial        World                `json:"initial"`
	Objectives     []Objective          `json:"objectives"`
}

type World struct {
	Entities []Entity     `json:"entities"`
	Accounts []Account    `json:"accounts"`
	Orders   []Order      `json:"orders"`
	Events   []WorldEvent `json:"events"`
	Ending   string       `json:"ending,omitempty"`
	Achieved []string     `json:"achieved"`
}

// An order survives quarter boundaries; amendments increment its version while
// original edicts and actual expenditure remain immutable journal evidence.
type Order struct {
	Disclosure    string           `json:"disclosure,omitempty"`
	ID            string           `json:"id"`
	Version       int              `json:"version"`
	EdictID       string           `json:"edict_id"`
	Kind          string           `json:"kind"`
	Text          string           `json:"text"`
	MinisterID    string           `json:"minister_id,omitempty"`
	Targets       []string         `json:"targets"`
	Condition     string           `json:"condition,omitempty"`
	DependsOn     []string         `json:"depends_on"`
	NotBeforeTurn int              `json:"not_before_turn"`
	Limits        map[string]int64 `json:"limits"`
	Spent         map[string]int64 `json:"spent"`
	Status        string           `json:"status"`
	Progress      int              `json:"progress"`
	LastResult    string           `json:"last_result"`
	CreatedTurn   int              `json:"created_turn"`
}

type OrderIntent struct {
	Disclosure    *string          `json:"disclosure,omitempty"`
	Action        string           `json:"action"`
	OrderID       string           `json:"order_id,omitempty"`
	Kind          string           `json:"kind,omitempty"`
	Text          string           `json:"text,omitempty"`
	MinisterID    string           `json:"minister_id,omitempty"`
	Targets       []string         `json:"targets,omitempty"`
	Condition     string           `json:"condition,omitempty"`
	DependsOn     []string         `json:"depends_on,omitempty"`
	NotBeforeTurn int              `json:"not_before_turn,omitempty"`
	Limits        map[string]int64 `json:"limits,omitempty"`
}

// A ledger entry moves one resource. Empty from/to denotes external income or
// consumption, with an explicit cause; internal transfers cannot create money.
type Transfer struct {
	CounterpartyHidden bool   `json:"counterparty_hidden,omitempty"`
	From               string `json:"from"`
	To                 string `json:"to"`
	Resource           string `json:"resource"`
	Amount             int64  `json:"amount"`
	OrderID            string `json:"order_id,omitempty"`
	Reason             string `json:"reason"`
}

type EntityChange struct {
	ID           string           `json:"id"`
	MetricDeltas map[string]int64 `json:"metric_deltas"`
	Attributes   map[string]Text  `json:"attributes"`
	Status       string           `json:"status,omitempty"`
	Location     *string          `json:"location,omitempty"`
	Owner        *string          `json:"owner,omitempty"`
	Reason       string           `json:"reason"`
}

type OrderResult struct {
	OrderID      string `json:"order_id"`
	Status       string `json:"status"`
	Progress     int    `json:"progress"`
	Reason       string `json:"reason"`
	ConditionMet bool   `json:"condition_met"`
}

type WorldProposal struct {
	NewAccounts []Account      `json:"new_accounts,omitempty"`
	Transfers   []Transfer     `json:"transfers"`
	Changes     []EntityChange `json:"changes"`
	NewEntities []Entity       `json:"new_entities"`
	Results     []OrderResult  `json:"results"`
	Events      []WorldEvent   `json:"events"`
}
