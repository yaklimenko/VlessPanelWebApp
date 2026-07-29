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
	ID      int    `json:"id"`
	Remark  string `json:"remark"`
	Port    int    `json:"port"`
	Protocol string `json:"protocol"`
	Settings string `json:"settings"`
}

// Client represents a client on a 3X-UI panel
type Client struct {
	ID         string   `json:"id"`
	Email      string   `json:"email"`
	Enable     bool     `json:"enable"`
	InboundIDs []int    `json:"inboundIds"`
	Inbounds   []string `json:"inbounds"`
	Keys       []VLESSKey `json:"keys,omitempty"`
}

// VLESSKey represents a VLESS connection key
type VLESSKey struct {
	Label    string `json:"label"`
	Protocol string `json:"protocol"`
	Link     string `json:"link"`
	Inbound  string `json:"inbound"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
	Security string `json:"security"`
	Transport string `json:"transport"`
}

// Subscription represents a named collection of VLESS keys
type Subscription struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Keys        []SubKey  `json:"keys"`
	Link        string    `json:"link,omitempty"`
	TestResults string    `json:"testResults,omitempty"`
}

// SubKey is a single key within a subscription
type SubKey struct {
	ID   string `json:"id"`
	Link string `json:"link"`
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

// CreateSubscriptionRequest is the request body for creating a subscription
type CreateSubscriptionRequest struct {
	Name string   `json:"name"`
	Keys []SubKey `json:"keys"`
}

// UpdateSubscriptionRequest is the request body for updating a subscription
type UpdateSubscriptionRequest struct {
	Name string   `json:"name,omitempty"`
	Keys []SubKey `json:"keys,omitempty"`
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
	Settings       json.RawMessage    `json:"settings"`
	StreamSettings *XUIStreamSettings `json:"streamSettings,omitempty"`
	ClientStats    []XUIClientStats   `json:"clientStats"`
}

// XUIStreamSettings represents streamSettings in a 3X-UI inbound
type XUIStreamSettings struct {
	Network         string             `json:"network"`
	Security        string             `json:"security"`
	RealitySettings *XUIRealitySettings `json:"realitySettings,omitempty"`
}

// XUIRealitySettings represents realitySettings in streamSettings
type XUIRealitySettings struct {
	ServerNames []string                 `json:"serverNames,omitempty"`
	ShortIds    []string                 `json:"shortIds,omitempty"`
	Settings    *XUIRealityClientSettings `json:"settings,omitempty"`
}

// XUIRealityClientSettings represents the nested settings inside realitySettings
type XUIRealityClientSettings struct {
	PublicKey   string `json:"publicKey,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	ServerName  string `json:"serverName,omitempty"`
	SpiderX     string `json:"spiderX,omitempty"`
}

// XUIResponse is a generic 3X-UI API response
type XUIResponse struct {
	Success bool        `json:"success"`
	Msg     string      `json:"msg"`
	Obj     interface{} `json:"obj,omitempty"`
}

// XUIClientSettings represents the settings JSON for a client
type XUIClientSettings struct {
	Email    string `json:"email"`
	Flow     string `json:"flow,omitempty"`
	ID       string `json:"id"`
	Level    int    `json:"level,omitempty"`
}
