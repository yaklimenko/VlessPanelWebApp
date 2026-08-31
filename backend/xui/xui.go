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
	Up             int64              `json:"up"` // кумулятивные счётчики инбаунда
	Down           int64              `json:"down"`
	Total          int64              `json:"total"`
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

// --- Телеметрия (статистика, раздел статистики) ---

// ServerStatus — живой снапшот системы с панели (GET /panel/api/server/status).
// Панель кэширует его ~раз в 2 секунды; поля — по OpenAPI 3.7.0.
type ServerStatus struct {
	CPU      float64    `json:"cpu"`      // %
	Mem      MemStats   `json:"mem"`      // current/total байт
	Swap     MemStats   `json:"swap"`     // current/total байт
	Disk     MemStats   `json:"disk"`     // current/total байт
	NetIO    NetIOStats `json:"netIO"`    // кумулятивные счётчики байт
	Xray     XrayState  `json:"xray"`     // состояние Xray
	TCPCount int        `json:"tcpCount"` // открытые соединения
	Load     LoadStats  `json:"load"`     // load average 1/5/15
}

// MemStats — счётчик current/total (RAM, swap или диск).
type MemStats struct {
	Current int64 `json:"current"`
	Total   int64 `json:"total"`
}

// NetIOStats — кумулятивные сетевые счётчики панели.
type NetIOStats struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// XrayState — состояние Xray из server/status.
type XrayState struct {
	State   string `json:"state"`
	Version string `json:"version"`
}

// LoadStats — load average 1/5/15.
type LoadStats struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

// HistoryPoint — одна точка истории панели (GET /panel/api/server/history/{metric}/{bucket}).
// t — unix-таймстемп (секунды, UTC), v — значение метрики в бакете.
type HistoryPoint struct {
	T int64   `json:"t"`
	V float64 `json:"v"`
}
