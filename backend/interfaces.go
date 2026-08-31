package main

import (
	"context"
	"time"

	"vlesspanel/dto"
	"vlesspanel/model"
	"vlesspanel/xui"
)

// Интерфейсы, от которых зависят сервисы (use-case слой). Конкретные
// реализации: *Storage (Repository), *PanelAPI (PanelClient),
// *vlessSubTestClient (VlessSubTestClient), *scriptSyncer (AggregatorSyncer).
// Благодаря интерфейсам сервисы можно юнит-тестировать с фейками.

// Repository — интерфейс файлового хранилища.
type Repository interface {
	// panels
	LoadPanels() ([]model.Panel, error)
	GetPanel(id string) (model.Panel, error)
	AddPanel(req dto.CreatePanelRequest) (model.Panel, error)
	UpdatePanelName(id, name string) (model.Panel, error)
	DeletePanel(id string) error

	// key sources
	LoadKeySources() ([]model.KeySource, error)
	GetKeySource(id string) (*model.KeySource, error)
	AddKeySource(ks model.KeySource) (*model.KeySource, bool, error)
	UpdateKeySource(ks model.KeySource) error
	UpdateKeySourceCaches(caches map[string]model.CachedKey) error
	DeleteKeySource(id string) error

	// subscriptions meta
	LoadSubscriptionsMeta() ([]model.Subscription, error)
	GetSubMeta(name string) (*model.Subscription, bool)
	UpsertSubMeta(sub model.Subscription) error
	DeleteSubMeta(name string) error
	ListSubscriptions() ([]model.Subscription, error)
	GetSubscription(name string) (*model.Subscription, error)

	// subscription files
	SubscriptionFileExists(name string) bool
	WriteSubscriptionFile(name string, keys []model.SubKey) error
	RenameSubscriptionFile(oldName, newName string) error
	RemoveSubscriptionFile(name string) error
	SubscriptionFileMtime(name string) string
	GetSubscriptionRaw(name string) (string, error)

	// tokens
	LoadTokens() ([]model.APIToken, error)
	AddToken(tok model.APIToken) error
	DeleteToken(id string) (model.APIToken, error)
}

// PanelClient — интерфейс клиента 3X-UI панели.
type PanelClient interface {
	ListClients(panel model.Panel) ([]model.Client, error)
	CreateClient(panel model.Panel, inboundID int, email, expiryDate string) error
	GetClientKeys(panel model.Panel, email string) ([]model.VLESSKey, error)
	ListInbounds(panel model.Panel) ([]xui.XUIInbound, error)
	AttachClient(panel model.Panel, email string, inboundID int) error
	DetachClient(panel model.Panel, email string, inboundID int) error
	UpdateClient(panel model.Panel, email string, expiryTime int64) error
	GetClientKeyForInbound(panel model.Panel, email string, inboundID int) (model.VLESSKey, error)
	GetClientStats(panel model.Panel, email string) (*model.Client, error)
	ListClientsAndInbounds(panel model.Panel) ([]model.Client, []xui.XUIInbound, error)
	GetClientKeysForEmails(panel model.Panel, emails []string, concurrency int) (map[string][]model.VLESSKey, []xui.XUIInbound, error)
}

// VlessSubTestClient — интерфейс демона тестов (vlesssubtest).
type VlessSubTestClient interface {
	Status() dto.VlessSubTestStatus
	TestSingle(vless string, timeout int) (dto.TestSingleResponse, error)
	ListRuns(from, to time.Time) ([]DaemonRun, error)
}

// AggregatorSyncer — интерфейс синка файлов с агрегатором.
type AggregatorSyncer interface {
	Sync(ctx context.Context) (string, error)
}
