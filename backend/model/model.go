// Package model содержит доменные сущности VlessPanel.
package model

// Panel represents a 3X-UI panel configuration
type Panel struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	URL                string `json:"url"`
	Token              string `json:"token,omitempty"`
	WebBasePath        string `json:"webBasePath,omitempty"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"` // true = пропустить проверку TLS (self-signed)
}

// Client represents a client on a 3X-UI panel
type Client struct {
	ID         string     `json:"id"`
	Email      string     `json:"email"`
	Enable     bool       `json:"enable"`
	ExpiryTime int64      `json:"expiryTime"`
	InboundIDs []int      `json:"inboundIds"`
	Inbounds   []string   `json:"inbounds"`
	Keys       []VLESSKey `json:"keys,omitempty"`
	Up         int64      `json:"up,omitempty"`
	Down       int64      `json:"down,omitempty"`
}

// VLESSKey represents a VLESS connection key
type VLESSKey struct {
	Label     string `json:"label"`
	Protocol  string `json:"protocol"`
	Link      string `json:"link"`
	Inbound   string `json:"inbound"`
	Server    string `json:"server"`
	Port      int    `json:"port"`
	Security  string `json:"security"`
	Transport string `json:"transport"`
}

// Subscription represents a named collection of VLESS keys
type Subscription struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"` // draft | active
	Keys        []SubKey `json:"keys"`
	Link        string   `json:"link,omitempty"`
	TestResults string   `json:"testResults,omitempty"`
	UpdatedAt   string   `json:"updatedAt,omitempty"`

	// Derived fields (computed at read time, not stored)
	FileMtime        string `json:"fileMtime,omitempty"`
	AggrLastModified string `json:"aggrLastModified,omitempty"`
	Synced           *bool  `json:"synced,omitempty"` // true=ok, false=changed, null=unknown
}

// SubKey is a single key within a subscription.
// KeySourceID is nullable: null means manual / legacy key (not tracked by KeySource).
type SubKey struct {
	ID          string  `json:"id"`
	Link        string  `json:"link"`
	KeySourceID *string `json:"keySourceId,omitempty"`
}

// KeySource is a source of a VLESS key: either a client@panel + inbound
// (type=panel, key fetched live from 3X-UI) or a raw vless link (type=manual).
type KeySource struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"` // panel | manual
	PanelID     string         `json:"panelId,omitempty"`
	ClientEmail string         `json:"clientEmail,omitempty"`
	InboundID   int            `json:"inboundId,omitempty"`
	VlessLink   string         `json:"vlessLink,omitempty"`
	Label       string         `json:"label,omitempty"`
	CachedKey   *CachedKey     `json:"cachedKey,omitempty"`
	LastTest    *KeySourceTest `json:"lastTest,omitempty"`
	CreatedAt   string         `json:"createdAt"`
	UpdatedAt   string         `json:"updatedAt"`

	// Derived fields (filled by handlers at read time)
	Status        string            `json:"status,omitempty"` // ok | expired | missing | panel_unreachable
	Error         string            `json:"error,omitempty"`
	ExpiryTime    int64             `json:"expiryTime,omitempty"` // ms epoch from panel
	ExpireDate    string            `json:"expireDate,omitempty"` // YYYY-MM-DD
	ClientEnabled bool              `json:"clientEnabled"`
	Traffic       *KeySourceTraffic `json:"traffic,omitempty"`
	PanelName     string            `json:"panelName,omitempty"`
	InboundRemark string            `json:"inboundRemark,omitempty"`
	InboundPort   int               `json:"inboundPort,omitempty"`
	KeyAvailable  bool              `json:"keyAvailable"` // fresh key was resolvable
	UsedInSubs    int               `json:"usedInSubs,omitempty"`
}

// CachedKey is a cached vless key for display (NOT the source of truth).
type CachedKey struct {
	Link      string `json:"link"`
	FetchedAt string `json:"fetchedAt"`
}

// KeySourceTest is the result of a test-single run for a KeySource.
type KeySourceTest struct {
	Status string `json:"status"` // ok | fail
	At     string `json:"at"`
	Ms     int    `json:"ms,omitempty"`
	Error  string `json:"error,omitempty"`
}

// KeySourceTraffic is the up/down traffic snapshot for a KeySource.
type KeySourceTraffic struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// GenerationReportItem is one row of a generation report (partial success).
type GenerationReportItem struct {
	Kind  string `json:"kind"` // ok | manual | skip
	Label string `json:"label"`
	Ms    int    `json:"ms,omitempty"`
	Why   string `json:"why,omitempty"`
}

// GenerationReport describes the outcome of a subscription generation.
type GenerationReport struct {
	Items    []GenerationReportItem `json:"report"`
	Included int                    `json:"included"`
	Skipped  int                    `json:"skipped"`
}

// APIToken represents an issued API token (for bots/agents). Only the SHA-256
// hash is persisted — the raw token is returned once at creation.
type APIToken struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	TokenHash string `json:"tokenHash"` // sha256 hex of the raw token (persisted, not exposed in API responses)
	CreatedAt string `json:"createdAt"`
}
