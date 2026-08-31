package dto

import (
	"encoding/json"
	"time"
)

// DaemonRun — одна запись прогона из bbolt демона vlesssubtest (зеркало
// RunRecord). Results — сырой JSON-массив per-key результатов ([]DaemonKeyResult).
// Общий тип для корневого backend (vlesssubtest.go) и подпакета backend/metrics
// (коллектор забирает прогоны через DaemonClient).
type DaemonRun struct {
	ID              string          `json:"id"`
	Kind            string          `json:"kind"` // "test" | "probe"
	SubscriptionURL string          `json:"subscription_url"`
	StartedAt       time.Time       `json:"started_at"`
	FinishedAt      time.Time       `json:"finished_at"`
	DurationSec     int             `json:"duration_sec"`
	Total           int             `json:"total"`
	OK              int             `json:"ok"`
	Degraded        int             `json:"degraded,omitempty"`
	Failed          int             `json:"failed"`
	Results         json.RawMessage `json:"results,omitempty"`
	Error           string          `json:"error,omitempty"`
}

// DaemonKeyResult — результат по одному ключу внутри прогона (TestResultItem демона).
type DaemonKeyResult struct {
	KeyIdx    int    `json:"key_idx"`
	IP        string `json:"ip,omitempty"`
	Remark    string `json:"remark,omitempty"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	Youtube   string `json:"youtube"`
	Instagram string `json:"instagram"`
}
