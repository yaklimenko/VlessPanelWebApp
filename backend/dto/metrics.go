// Package dto содержит request/response DTO слоя сервисов.
package dto

// --- Раздел статистики (Этап 1: коллектор + БД + API метрик) ---

// MetricsSnapshotPoint — агрегированная точка снапшота панели за бакет.
// Поля-указатели: null = данных в бакете нет (пропуск из-за недоступности).
type MetricsSnapshotPoint struct {
	TS             int64    `json:"ts"`
	CPUAvg         *float64 `json:"cpuAvg"`
	CPUMax         *float64 `json:"cpuMax"`
	MemAvg         *float64 `json:"memAvg"`
	MemMax         *float64 `json:"memMax"`
	SwapAvg        *float64 `json:"swapAvg"`
	Load1Avg       *float64 `json:"load1Avg"`
	Load5Avg       *float64 `json:"load5Avg"`
	Load15Avg      *float64 `json:"load15Avg"`
	NetUp          *int64   `json:"netUp"`          // байты за бакет
	NetDown        *int64   `json:"netDown"`        // байты за бакет
	NetTrafficSent *int64   `json:"netTrafficSent"` // кумулятивный счётчик (контрольная сумма)
	NetTrafficRecv *int64   `json:"netTrafficRecv"` // кумулятивный счётчик (контрольная сумма)
	DiskUsed       *int64   `json:"diskUsed"`
	DiskTotal      *int64   `json:"diskTotal"`
	OnlineAvg      *int     `json:"onlineAvg"`
	OnlineMax      *int     `json:"onlineMax"`
	OpenConnsMax   *int     `json:"openConnsMax"`
	XrayOK         int      `json:"xrayOk"` // 1 = xray running, 0 = нет
}

// MetricsSnapshotResponse — ответ GET /api/metrics/snapshots.
type MetricsSnapshotResponse struct {
	PanelID       string                 `json:"panelId"`
	Range         string                 `json:"range"` // 24h | 7d | 90d
	BucketSeconds int64                  `json:"bucketSeconds"`
	Points        []MetricsSnapshotPoint `json:"points"`
}

// InboundTrafficPoint — дельта трафика инбаунда за бакет.
type InboundTrafficPoint struct {
	TS        int64  `json:"ts"`
	InboundID int    `json:"inboundId"`
	Remark    string `json:"remark"`
	UpDelta   int64  `json:"upDelta"` // байты за бакет
	DownDelta int64  `json:"downDelta"`
}

// ClientTrafficPoint — дельта трафика клиента за бакет.
type ClientTrafficPoint struct {
	TS        int64  `json:"ts"`
	InboundID int    `json:"inboundId"`
	Email     string `json:"email"`
	UpDelta   int64  `json:"upDelta"`
	DownDelta int64  `json:"downDelta"`
}

// MetricsTrafficResponse — ответ GET /api/metrics/traffic.
type MetricsTrafficResponse struct {
	PanelID       string      `json:"panelId"`
	Range         string      `json:"range"`
	BucketSeconds int64       `json:"bucketSeconds"`
	GroupBy       string      `json:"groupBy"` // inbound | client
	Points        interface{} `json:"points"`
}

// MetricsTestRun — прогон теста подписки (test_runs + tester).
type MetricsTestRun struct {
	ID             int64              `json:"id"`
	TesterID       int64              `json:"testerId"`
	TesterName     string             `json:"testerName"`
	SubscriptionID string             `json:"subscriptionId"`
	Status         string             `json:"status"` // ok | partial | failed | running
	Total          int                `json:"total"`
	OKCount        int                `json:"okCount"`
	FailCount      int                `json:"failCount"`
	Error          string             `json:"error,omitempty"`
	StartedAt      string             `json:"startedAt"`
	FinishedAt     string             `json:"finishedAt,omitempty"`
	Results        []MetricsKeyResult `json:"results,omitempty"` // только в detail
}

// MetricsKeyResult — результат по ключу внутри прогона (test_key_results).
type MetricsKeyResult struct {
	ID                int64   `json:"id"`
	KeyID             string  `json:"keyId,omitempty"` // id SubKey из VlessPanel (связь с подпиской)
	Label             string  `json:"label"`
	Status            string  `json:"status"` // OK | FAIL | TIMEOUT | ERROR
	IP                string  `json:"ip,omitempty"`
	LatencyMs         *int    `json:"latencyMs,omitempty"`
	AvgSpeedKbps      float64 `json:"avgSpeedKbps,omitempty"`
	StabilityPct      float64 `json:"stabilityPct,omitempty"`
	Reconnects        int     `json:"reconnects,omitempty"`
	TotalDownloadedMB float64 `json:"totalDownloadedMb,omitempty"`
	SessionsOK        int     `json:"sessionsOk,omitempty"`
	SessionsFail      int     `json:"sessionsFail,omitempty"`
	DurationSec       int     `json:"durationSec,omitempty"`
	TestedAt          string  `json:"testedAt"`
}

// MetricsTestRunsResponse — ответ GET /api/metrics/test-runs.
type MetricsTestRunsResponse struct {
	Range string           `json:"range"`
	Runs  []MetricsTestRun `json:"runs"`
}

// MetricsAvailability — последний снапшот по панели (сигнал недоступности:
// свежих данных нет → панель не отдаёт телеметрию).
type MetricsAvailability struct {
	PanelID        string `json:"panelId"`
	Name           string `json:"name"`
	LastSnapshotTS int64  `json:"lastSnapshotTs"` // 0 = данных ещё не было
}

// MetricsTester — тестер из реестра.
type MetricsTester struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	BaseURL         string  `json:"baseUrl"`
	Location        string  `json:"location,omitempty"`
	Enabled         int     `json:"enabled"`
	Weight          int     `json:"weight"`
	Priority        int     `json:"priority"`
	LastHeartbeatAt *string `json:"lastHeartbeatAt,omitempty"`
}
