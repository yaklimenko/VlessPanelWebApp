package main

import "encoding/json"

// Panel represents a 3X-UI panel configuration
type Panel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Token       string `json:"token,omitempty"`
	WebBasePath string `json:"webBasePath,omitempty"`
}

// Inbound represents a 3X-UI inbound
type Inbound struct {
	ID       int    `json:"id"`
	Remark   string `json:"remark"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Settings string `json:"settings"`
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

// CreateKeySourceRequest is the request body for POST /api/key-sources
type CreateKeySourceRequest struct {
	Type        string `json:"type"` // panel | manual
	PanelID     string `json:"panelId,omitempty"`
	ClientEmail string `json:"clientEmail,omitempty"`
	InboundID   int    `json:"inboundId,omitempty"`
	VlessLink   string `json:"vlessLink,omitempty"`
	Label       string `json:"label,omitempty"`
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

// SubscriptionGenerateResponse is the response for create/regenerate.
type SubscriptionGenerateResponse struct {
	Subscription Subscription           `json:"subscription"`
	Report       []GenerationReportItem `json:"report"`
	Included     int                    `json:"included"`
	Skipped      int                    `json:"skipped"`
}

// UpdateSubscriptionRequest is the request body for PUT /api/subscriptions/{id}
type UpdateSubscriptionRequest struct {
	Name string   `json:"name,omitempty"`
	Keys []SubKey `json:"keys,omitempty"` // legacy full-replacement mode

	// KeySource modes:
	AddKeySourceIDs []string `json:"addKeySourceIds,omitempty"` // add KeySource refs (no file write)
	RemoveKeyID     string   `json:"removeKeyId,omitempty"`     // remove one SubKey + rewrite file
	Regenerate      bool     `json:"regenerate,omitempty"`      // re-fetch fresh keys, keep manual
}

// SyncCheckResult is the result of a HEAD check against the aggregator.
type SyncCheckResult struct {
	Name             string `json:"name"`
	AggrLastModified string `json:"aggrLastModified,omitempty"`
	LocalMtime       string `json:"localMtime,omitempty"`
	Synced           *bool  `json:"synced,omitempty"`
	Error            string `json:"error,omitempty"`
}

// CreatePanelRequest is the request body for adding a panel
type CreatePanelRequest struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Token       string `json:"token"`
	WebBasePath string `json:"webBasePath,omitempty"`
}

// CreateClientRequest is the request body for creating a client
type CreateClientRequest struct {
	Email      string `json:"email"`
	InboundID  int    `json:"inboundId"`
	ExpiryDate string `json:"expiryDate,omitempty"`
}

// UpdateClientRequest is the request body for updating a client
type UpdateClientRequest struct {
	ExpiryDate string `json:"expiryDate,omitempty"`
}

// CreateSubscriptionRequest is the request body for creating a subscription
// (KeySource mode: keySourceIds; legacy mode: keys)
type CreateSubscriptionRequest struct {
	Name         string   `json:"name"`
	Keys         []SubKey `json:"keys,omitempty"`
	KeySourceIDs []string `json:"keySourceIds,omitempty"`
}

// TestSingleRequest is the body for daemon's POST /test-single
type TestSingleRequest struct {
	Vless   string `json:"vless"`
	Timeout int    `json:"timeout,omitempty"`
}

// TestSingleResponse is the daemon's response from POST /test-single
type TestSingleResponse struct {
	KeyIdx    int    `json:"key_idx"`
	IP        string `json:"ip"`
	Remark    string `json:"remark"`
	Status    string `json:"status"`
	YouTube   string `json:"youtube"`
	Instagram string `json:"instagram"`
}

// TestSubscriptionResponse is the API response for subscription test results
type TestSubscriptionResponse struct {
	Total   int                  `json:"total"`
	OK      int                  `json:"ok"`
	Results []TestSingleResponse `json:"results"`
}

// XUIClientStats represents a client from 3X-UI API (tested with v3.4.2)
type XUIClientStats struct {
	ID         int    `json:"id"`
	Email      string `json:"email"`
	Enable     bool   `json:"enable"`
	Up         int64  `json:"up"`
	Down       int64  `json:"down"`
	Total      int64  `json:"total"`
	ExpiryTime int64  `json:"expiryTime"`
}

// XUIClientTraffic represents nested traffic stats in /panel/api/clients/list
type XUIClientTraffic struct {
	Up     int64 `json:"up"`
	Down   int64 `json:"down"`
	Enable bool  `json:"enable"`
}

// XUIClient represents a client from /panel/api/clients/list (3X-UI v3.4.2+)
type XUIClient struct {
	ID         int              `json:"id"`
	Email      string           `json:"email"`
	Enable     bool             `json:"enable"`
	ExpiryTime int64            `json:"expiryTime"`
	TotalGB    int64            `json:"totalGB"`
	InboundIDs []int            `json:"inboundIds"`
	Traffic    XUIClientTraffic `json:"traffic"`
}

// XUIInbound represents an inbound from 3X-UI API (tested with v3.4.2)
type XUIInbound struct {
	ID             int                `json:"id"`
	Remark         string             `json:"remark"`
	Port           int                `json:"port"`
	Protocol       string             `json:"protocol"`
	Enable         bool               `json:"enable"`
	Settings       json.RawMessage    `json:"settings"`
	StreamSettings *XUIStreamSettings `json:"streamSettings,omitempty"`
	ClientStats    []XUIClientStats   `json:"clientStats"`
}

// XUIStreamSettings represents streamSettings in a 3X-UI inbound
type XUIStreamSettings struct {
	Network         string              `json:"network"`
	Security        string              `json:"security"`
	RealitySettings *XUIRealitySettings `json:"realitySettings,omitempty"`
}

// XUIRealitySettings represents realitySettings in streamSettings
type XUIRealitySettings struct {
	ServerNames []string                  `json:"serverNames,omitempty"`
	ShortIds    []string                  `json:"shortIds,omitempty"`
	Settings    *XUIRealityClientSettings `json:"settings,omitempty"`
}

// XUIRealityClientSettings represents the nested settings inside realitySettings
type XUIRealityClientSettings struct {
	PublicKey     string `json:"publicKey,omitempty"`
	Fingerprint   string `json:"fingerprint,omitempty"`
	ServerName    string `json:"serverName,omitempty"`
	SpiderX       string `json:"spiderX,omitempty"`
	Mldsa65Verify string `json:"mldsa65Verify,omitempty"`
}

// XUIResponse is a generic 3X-UI API response
type XUIResponse struct {
	Success bool        `json:"success"`
	Msg     string      `json:"msg"`
	Obj     interface{} `json:"obj,omitempty"`
}

// XUIClientSettings represents the settings JSON for a client
type XUIClientSettings struct {
	Email string `json:"email"`
	Flow  string `json:"flow,omitempty"`
	ID    string `json:"id"`
	Level int    `json:"level,omitempty"`
}

// APIToken represents an issued API token (for bots/agents). Only the SHA-256
// hash is persisted — the raw token is returned once at creation.
type APIToken struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	TokenHash string `json:"tokenHash"` // sha256 hex of the raw token (persisted, not exposed in API responses)
	CreatedAt string `json:"createdAt"`
}

// CreateTokenRequest is the request body for POST /api/tokens.
type CreateTokenRequest struct {
	Label string `json:"label"`
}

// CreateTokenResponse is the response for POST /api/tokens. The raw token is
// shown only once.
type CreateTokenResponse struct {
	Token    string   `json:"token"`
	APIToken APIToken `json:"tokenMeta"`
}
