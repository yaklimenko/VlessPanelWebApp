// Package dto содержит request/response DTO слоя сервисов.
package dto

import "vlesspanel/model"

// CreateKeySourceRequest is the request body for POST /api/key-sources
type CreateKeySourceRequest struct {
	Type        string `json:"type"` // panel | manual
	PanelID     string `json:"panelId,omitempty"`
	ClientEmail string `json:"clientEmail,omitempty"`
	InboundID   int    `json:"inboundId,omitempty"`
	VlessLink   string `json:"vlessLink,omitempty"`
	Label       string `json:"label,omitempty"`
}

// SubscriptionGenerateResponse is the response for create/regenerate.
type SubscriptionGenerateResponse struct {
	Subscription model.Subscription           `json:"subscription"`
	Report       []model.GenerationReportItem `json:"report"`
	Included     int                          `json:"included"`
	Skipped      int                          `json:"skipped"`
}

// UpdateSubscriptionRequest is the request body for PUT /api/subscriptions/{id}
type UpdateSubscriptionRequest struct {
	Name string         `json:"name,omitempty"`
	Keys []model.SubKey `json:"keys,omitempty"` // legacy full-replacement mode

	// KeySource modes:
	AddKeySourceIDs []string `json:"addKeySourceIds,omitempty"` // add KeySource refs (no file write)
	RemoveKeyID     string   `json:"removeKeyId,omitempty"`     // remove one SubKey + rewrite file
	Regenerate      bool     `json:"regenerate,omitempty"`      // re-fetch fresh keys, keep manual
}

// CreatePanelRequest is the request body for adding a panel
type CreatePanelRequest struct {
	Name               string `json:"name"`
	URL                string `json:"url"`
	Token              string `json:"token"`
	WebBasePath        string `json:"webBasePath,omitempty"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
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
	Name         string         `json:"name"`
	Keys         []model.SubKey `json:"keys,omitempty"`
	KeySourceIDs []string       `json:"keySourceIds,omitempty"`
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

// CreateTokenRequest is the request body for POST /api/tokens.
type CreateTokenRequest struct {
	Label string `json:"label"`
}

// CreateTokenResponse is the response for POST /api/tokens. The raw token is
// shown only once.
type CreateTokenResponse struct {
	Token    string         `json:"token"`
	APIToken model.APIToken `json:"tokenMeta"`
}

// StatusResponse — простой ack для операций с клиентами/панелями/токенами.
type StatusResponse struct {
	Status string `json:"status"`
	Email  string `json:"email,omitempty"`
	ID     string `json:"id,omitempty"`
}

// SimpleInbound — упрощённое представление инбаунда для ListInbounds.
type SimpleInbound struct {
	ID       int    `json:"id"`
	Remark   string `json:"remark"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Enable   bool   `json:"enable"`
}

// UpdateSubscriptionResult — результат UpdateSubscription. Generate не nil
// только для режима regenerate.
type UpdateSubscriptionResult struct {
	Subscription model.Subscription
	Generate     *SubscriptionGenerateResponse
}

// RegenerateSubResult — одна строка отчёта RegenerateAll.
type RegenerateSubResult struct {
	Name        string `json:"name"`
	Regenerated bool   `json:"regenerated"`
	Reason      string `json:"reason,omitempty"`
	Included    int    `json:"included,omitempty"`
	SkippedKeys int    `json:"skippedKeys,omitempty"`
}

// RegenerateAllResponse — отчёт RegenerateAllSubscriptions.
type RegenerateAllResponse struct {
	Regenerated int                   `json:"regenerated"`
	Skipped     int                   `json:"skipped"`
	Results     []RegenerateSubResult `json:"results"`
}

// CreateKeySourceResponse — ответ CreateKeySource (dedup или создание).
type CreateKeySourceResponse struct {
	KeySource *model.KeySource `json:"keySource"`
	Deduped   bool             `json:"deduped"`
}

// DeleteKeySourceResponse — отчёт DeleteKeySource (каскадная чистка).
type DeleteKeySourceResponse struct {
	Status              string   `json:"status"`
	Label               string   `json:"label"`
	UsedInSubscriptions int      `json:"usedInSubscriptions"`
	Subscriptions       []string `json:"subscriptions"`
}

// KeySourceKeyResponse — ответ GetKeySourceKey (свежий ключ + обновлённый source).
type KeySourceKeyResponse struct {
	Key    model.VLESSKey   `json:"key"`
	Source *model.KeySource `json:"source"`
}

// KeySourceTestResponse — ответ TestKeySource.
type KeySourceTestResponse struct {
	Result   *TestSingleResponse  `json:"result"`
	LastTest *model.KeySourceTest `json:"lastTest"`
	Error    string               `json:"error,omitempty"`
}

// KeySourceTrafficResponse — ответ GetKeySourceTraffic.
type KeySourceTrafficResponse struct {
	Up         int64 `json:"up"`
	Down       int64 `json:"down"`
	ExpiryTime int64 `json:"expiryTime"`
	Enable     bool  `json:"enable"`
}

// VlessSubTestStatus — ответ GetVlessSubTestStatus.
type VlessSubTestStatus struct {
	Available bool   `json:"available"`
	DaemonURL string `json:"daemonURL"`
	Error     string `json:"error,omitempty"`
}

// SyncResponse — ответ SyncToAggregator.
type SyncResponse struct {
	Status string `json:"status"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}
