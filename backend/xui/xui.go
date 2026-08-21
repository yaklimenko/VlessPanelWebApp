// Package xui содержит структуры данных API 3X-UI.
package xui

import "encoding/json"

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
