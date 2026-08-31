package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"vlesspanel/dto"
)

// vlessSubTestClient — HTTP-клиент к демону vlesssubtest.
type vlessSubTestClient struct {
	baseURL string
}

func NewVlessSubTestClient(baseURL string) *vlessSubTestClient {
	return &vlessSubTestClient{baseURL: strings.TrimRight(baseURL, "/")}
}

// Status возвращает статус демона (GET /test).
func (c *vlessSubTestClient) Status() dto.VlessSubTestStatus {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(c.baseURL + "/test")
	if err != nil {
		return dto.VlessSubTestStatus{Available: false, DaemonURL: c.baseURL, Error: err.Error()}
	}
	resp.Body.Close()
	return dto.VlessSubTestStatus{Available: true, DaemonURL: c.baseURL}
}

// TestSingle прогоняет один vless-ключ через демон (POST /test-single).
// Ошибки сети/разбора оборачиваются sentinel'ами для различения на уровне сервисов.
func (c *vlessSubTestClient) TestSingle(vless string, timeout int) (dto.TestSingleResponse, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	reqBody, _ := json.Marshal(dto.TestSingleRequest{Vless: vless, Timeout: timeout})

	resp, err := client.Post(c.baseURL+"/test-single", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return dto.TestSingleResponse{}, fmt.Errorf("%w: %v", ErrDaemonUnreachable, err)
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var single dto.TestSingleResponse
	if err := json.Unmarshal(body, &single); err != nil {
		return dto.TestSingleResponse{}, fmt.Errorf("%w: %v", ErrDaemonParse, err)
	}
	return single, nil
}

// --- Забор результатов прогонов (GET /runs) ---

// DaemonRun — одна запись прогона из bbolt демона (зеркало RunRecord).
// Results — сырой JSON-массив per-key результатов ([]DaemonKeyResult).
// Тип переехал в vlesspanel/dto (daemon.go) — общий для корневого backend и
// подпакета backend/metrics; алиас оставлен, чтобы не менять точки вызова.
type DaemonRun = dto.DaemonRun

// DaemonKeyResult — результат по одному ключу внутри прогона (TestResultItem демона).
type DaemonKeyResult = dto.DaemonKeyResult

// daemonRunsResponse — ответ GET /runs.
type daemonRunsResponse struct {
	Total int         `json:"total"`
	Runs  []DaemonRun `json:"runs"`
}

// ListRuns забирает прогоны демона за полуинтервал [from, to) с per-key
// деталями (detail=1). Ошибки сети оборачиваются в ErrDaemonUnreachable,
// ошибки разбора — в ErrDaemonParse.
func (c *vlessSubTestClient) ListRuns(from, to time.Time) ([]DaemonRun, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	q := url.Values{}
	q.Set("from", from.UTC().Format("2006-01-02T15:04:05"))
	q.Set("to", to.UTC().Format("2006-01-02T15:04:05"))
	q.Set("detail", "1")
	q.Set("limit", "1000")

	resp, err := client.Get(c.baseURL + "/runs?" + q.Encode())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDaemonUnreachable, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDaemonParse, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: daemon HTTP %d: %s", ErrDaemonParse, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out daemonRunsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDaemonParse, err)
	}
	if out.Runs == nil {
		out.Runs = []DaemonRun{}
	}
	return out.Runs, nil
}
