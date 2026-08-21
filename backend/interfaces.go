package main

import "context"

// Интерфейсы, от которых зависят сервисы (use-case слой). Конкретные
// реализации: *Storage (Repository), *PanelAPI (PanelClient),
// *vlessSubTestClient (VlessSubTestClient), *scriptSyncer (AggregatorSyncer).
// Благодаря интерфейсам сервисы можно юнит-тестировать с фейками.

// Repository — интерфейс файлового хранилища.
type Repository interface {
	// panels
	LoadPanels() ([]Panel, error)
	GetPanel(id string) (Panel, error)
	AddPanel(req CreatePanelRequest) (Panel, error)
	DeletePanel(id string) error

	// key sources
	LoadKeySources() ([]KeySource, error)
	GetKeySource(id string) (*KeySource, error)
	AddKeySource(ks KeySource) (*KeySource, bool, error)
	UpdateKeySource(ks KeySource) error
	UpdateKeySourceCaches(caches map[string]CachedKey) error
	DeleteKeySource(id string) error

	// subscriptions meta
	LoadSubscriptionsMeta() ([]Subscription, error)
	GetSubMeta(name string) (*Subscription, bool)
	UpsertSubMeta(sub Subscription) error
	DeleteSubMeta(name string) error
	ListSubscriptions() ([]Subscription, error)
	GetSubscription(name string) (*Subscription, error)

	// subscription files
	SubscriptionFileExists(name string) bool
	WriteSubscriptionFile(name string, keys []SubKey) error
	RenameSubscriptionFile(oldName, newName string) error
	RemoveSubscriptionFile(name string) error
	SubscriptionFileMtime(name string) string
	GetSubscriptionRaw(name string) (string, error)

	// tokens
	LoadTokens() ([]APIToken, error)
	AddToken(tok APIToken) error
	DeleteToken(id string) (APIToken, error)
}

// PanelClient — интерфейс клиента 3X-UI панели.
type PanelClient interface {
	ListClients(panel Panel) ([]Client, error)
	CreateClient(panel Panel, inboundID int, email, expiryDate string) error
	GetClientKeys(panel Panel, email string) ([]VLESSKey, error)
	ListInbounds(panel Panel) ([]XUIInbound, error)
	AttachClient(panel Panel, email string, inboundID int) error
	DetachClient(panel Panel, email string, inboundID int) error
	UpdateClient(panel Panel, email string, expiryTime int64) error
	GetClientKeyForInbound(panel Panel, email string, inboundID int) (VLESSKey, error)
	GetClientStats(panel Panel, email string) (*Client, error)
	ListClientsAndInbounds(panel Panel) ([]Client, []XUIInbound, error)
	GetClientKeysForEmails(panel Panel, emails []string, concurrency int) (map[string][]VLESSKey, []XUIInbound, error)
}

// VlessSubTestClient — интерфейс демона тестов (vlesssubtest).
type VlessSubTestClient interface {
	Status() VlessSubTestStatus
	TestSingle(vless string, timeout int) (TestSingleResponse, error)
}

// AggregatorSyncer — интерфейс синка файлов с агрегатором.
type AggregatorSyncer interface {
	Sync(ctx context.Context) (string, error)
}
