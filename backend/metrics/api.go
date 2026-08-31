package metrics

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"vlesspanel/dto"

	"github.com/go-chi/chi/v5"
)

// respondJSON/respondError — копии одноимённых хелперов из handlers.go корневого
// backend: подпакет metrics standalone (не импортирует корневой пакет), поэтому
// HTTP-ответы формирует сам. Поведение идентично.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

// MetricsHandlers — HTTP-транспорт раздела статистики (Этап 1, без UI).
// Отдельная структура от Handlers: зависит только от MetricsDB, легко тестировать.
type MetricsHandlers struct {
	db *MetricsDB
}

func NewMetricsHandlers(db *MetricsDB) *MetricsHandlers {
	return &MetricsHandlers{db: db}
}

// metricsRange — выбранный диапазон: [from, to] unix + размер бакета агрегации.
// 24h → сырые 5-минутные окна; 7d → часовые бакеты; 90d → 6-часовые.
type metricsRange struct {
	key       string
	from, to  int64
	bucketSec int64
}

func parseMetricsRange(q string) metricsRange {
	now := time.Now().UTC()
	to := now.Unix()
	switch q {
	case "7d":
		return metricsRange{"7d", now.AddDate(0, 0, -7).Unix(), to, 3600}
	case "90d":
		return metricsRange{"90d", now.AddDate(0, 0, -90).Unix(), to, 21600}
	default:
		return metricsRange{"24h", now.Add(-24 * time.Hour).Unix(), to, 300}
	}
}

// --- Тестеры ---

// Testers — GET /api/metrics/testers — реестр тестировщиков.
func (h *MetricsHandlers) Testers(w http.ResponseWriter, r *http.Request) {
	testers, err := h.db.ListTesters()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]dto.MetricsTester, 0, len(testers))
	for _, t := range testers {
		out = append(out, dto.MetricsTester{
			ID:              t.ID,
			Name:            t.Name,
			BaseURL:         t.BaseURL,
			Location:        t.Location,
			Enabled:         t.Enabled,
			Weight:          t.Weight,
			Priority:        t.Priority,
			LastHeartbeatAt: t.LastHeartbeatAt,
		})
	}
	respondJSON(w, http.StatusOK, out)
}

// --- Снапшоты панелей ---

// Snapshots — GET /api/metrics/snapshots?range=24h|7d|90d&panelId=<id>
func (h *MetricsHandlers) Snapshots(w http.ResponseWriter, r *http.Request) {
	panelID := r.URL.Query().Get("panelId")
	if panelID == "" {
		respondError(w, http.StatusBadRequest, "panelId is required")
		return
	}
	rg := parseMetricsRange(r.URL.Query().Get("range"))

	rows, err := h.db.Snapshots(panelID, rg.from, rg.to)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := dto.MetricsSnapshotResponse{
		PanelID:       panelID,
		Range:         rg.key,
		BucketSeconds: rg.bucketSec,
		Points:        aggregateSnapshots(rows, rg.bucketSec),
	}
	respondJSON(w, http.StatusOK, resp)
}

// aggregateSnapshots схлопывает сырые 5-минутные строки в бакеты:
// avg-поля — среднее, max-поля — максимум, net_up/down — сумма по бакету,
// кумулятивные счётчики и диск — значение последней строки бакета.
func aggregateSnapshots(rows []SnapshotRow, bucketSec int64) []dto.MetricsSnapshotPoint {
	if len(rows) == 0 {
		return []dto.MetricsSnapshotPoint{}
	}
	var out []dto.MetricsSnapshotPoint

	flush := func(bucketTS int64, group []SnapshotRow) dto.MetricsSnapshotPoint {
		p := dto.MetricsSnapshotPoint{TS: bucketTS, XrayOK: 1}
		var cpuSum, memSum, swapSum, l1Sum, l5Sum, l15Sum float64
		var cpuN, memN, swapN, l1N, l5N, l15N int
		var onlineSum float64
		var onlineN int
		var netUp, netDown int64
		var xrayAnyFail bool

		for _, s := range group {
			if s.CPUAvg != nil {
				cpuSum += *s.CPUAvg
				cpuN++
			}
			if s.CPUMax != nil && (p.CPUMax == nil || *s.CPUMax > *p.CPUMax) {
				p.CPUMax = s.CPUMax
			}
			if s.MemAvg != nil {
				memSum += *s.MemAvg
				memN++
			}
			if s.MemMax != nil && (p.MemMax == nil || *s.MemMax > *p.MemMax) {
				p.MemMax = s.MemMax
			}
			if s.SwapAvg != nil {
				swapSum += *s.SwapAvg
				swapN++
			}
			if s.Load1Avg != nil {
				l1Sum += *s.Load1Avg
				l1N++
			}
			if s.Load5Avg != nil {
				l5Sum += *s.Load5Avg
				l5N++
			}
			if s.Load15Avg != nil {
				l15Sum += *s.Load15Avg
				l15N++
			}
			if s.NetUp != nil {
				netUp += *s.NetUp
			}
			if s.NetDown != nil {
				netDown += *s.NetDown
			}
			if s.OnlineAvg != nil {
				onlineSum += float64(*s.OnlineAvg)
				onlineN++
			}
			if s.OnlineMax != nil && (p.OnlineMax == nil || *s.OnlineMax > *p.OnlineMax) {
				p.OnlineMax = s.OnlineMax
			}
			if s.OpenConnsMax != nil && (p.OpenConnsMax == nil || *s.OpenConnsMax > *p.OpenConnsMax) {
				p.OpenConnsMax = s.OpenConnsMax
			}
			if s.XrayOK == 0 {
				xrayAnyFail = true
			}
			// «Последняя строка бакета»: строки идут по ts ↑, перезаписываем.
			p.NetTrafficSent = s.NetTrafficSent
			p.NetTrafficRecv = s.NetTrafficRecv
			p.DiskUsed = s.DiskUsed
			p.DiskTotal = s.DiskTotal
		}

		if cpuN > 0 {
			v := cpuSum / float64(cpuN)
			p.CPUAvg = &v
		}
		if memN > 0 {
			v := memSum / float64(memN)
			p.MemAvg = &v
		}
		if swapN > 0 {
			v := swapSum / float64(swapN)
			p.SwapAvg = &v
		}
		if l1N > 0 {
			v := l1Sum / float64(l1N)
			p.Load1Avg = &v
		}
		if l5N > 0 {
			v := l5Sum / float64(l5N)
			p.Load5Avg = &v
		}
		if l15N > 0 {
			v := l15Sum / float64(l15N)
			p.Load15Avg = &v
		}
		if onlineN > 0 {
			v := int(math.Round(onlineSum / float64(onlineN)))
			p.OnlineAvg = &v
		}
		p.NetUp = int64Ptr(netUp)
		p.NetDown = int64Ptr(netDown)
		if xrayAnyFail {
			p.XrayOK = 0
		}
		return p
	}

	curBucket := rows[0].TS / bucketSec * bucketSec
	group := []SnapshotRow{rows[0]}
	for _, s := range rows[1:] {
		b := s.TS / bucketSec * bucketSec
		if b != curBucket {
			out = append(out, flush(curBucket, group))
			curBucket = b
			group = nil
		}
		group = append(group, s)
	}
	out = append(out, flush(curBucket, group))
	return out
}

// --- Трафик ---

// Traffic — GET /api/metrics/traffic?range=&panelId=&groupBy=inbound|client
// Инбаунды: дельты счётчиков между срезами (считаем на чтении, как в заметке).
// Клиенты: сумма delta-записей по бакетам.
func (h *MetricsHandlers) Traffic(w http.ResponseWriter, r *http.Request) {
	panelID := r.URL.Query().Get("panelId")
	if panelID == "" {
		respondError(w, http.StatusBadRequest, "panelId is required")
		return
	}
	groupBy := r.URL.Query().Get("groupBy")
	if groupBy != "client" {
		groupBy = "inbound"
	}
	rg := parseMetricsRange(r.URL.Query().Get("range"))

	resp := dto.MetricsTrafficResponse{
		PanelID:       panelID,
		Range:         rg.key,
		BucketSeconds: rg.bucketSec,
		GroupBy:       groupBy,
	}

	if groupBy == "client" {
		rows, err := h.db.ClientTrafficRows(panelID, rg.from, rg.to)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp.Points = aggregateClientTraffic(rows, rg.bucketSec)
	} else {
		rows, err := h.db.InboundTrafficRows(panelID, rg.from, rg.to)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp.Points = aggregateInboundTraffic(rows, rg.bucketSec)
	}
	respondJSON(w, http.StatusOK, resp)
}

// aggregateInboundTraffic считает дельты счётчиков между соседними срезами
// каждого инбаунда и суммирует их по бакетам. Первый срез инбаунда — база,
// дельты не даёт; сброс счётчика (отрицательная дельта) — 0.
func aggregateInboundTraffic(rows []InboundTrafficRecord, bucketSec int64) []dto.InboundTrafficPoint {
	var out []dto.InboundTrafficPoint
	if len(rows) == 0 {
		return out
	}

	prev := map[int]InboundTrafficRecord{} // inbound_id → прошлый срез
	acc := map[int64]map[int]*dto.InboundTrafficPoint{}

	for _, r := range rows {
		if prevRec, ok := prev[r.InboundID]; ok {
			up := r.Up - prevRec.Up
			down := r.Down - prevRec.Down
			if up < 0 {
				up = 0
			}
			if down < 0 {
				down = 0
			}
			if up == 0 && down == 0 {
				prev[r.InboundID] = r
				continue
			}
			b := r.TS / bucketSec * bucketSec
			bucketMap := acc[b]
			if bucketMap == nil {
				bucketMap = map[int]*dto.InboundTrafficPoint{}
				acc[b] = bucketMap
			}
			pt := bucketMap[r.InboundID]
			if pt == nil {
				pt = &dto.InboundTrafficPoint{TS: b, InboundID: r.InboundID, Remark: r.Remark}
				bucketMap[r.InboundID] = pt
			}
			pt.UpDelta += up
			pt.DownDelta += down
		}
		prev[r.InboundID] = r
	}

	for _, m := range acc {
		for _, pt := range m {
			out = append(out, *pt)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TS != out[j].TS {
			return out[i].TS < out[j].TS
		}
		if out[i].InboundID != out[j].InboundID {
			return out[i].InboundID < out[j].InboundID
		}
		return out[i].Remark < out[j].Remark
	})
	return out
}

// aggregateClientTraffic суммирует delta-записи клиентов по (бакет, инбаунд, email).
func aggregateClientTraffic(rows []ClientTrafficRecord, bucketSec int64) []dto.ClientTrafficPoint {
	var out []dto.ClientTrafficPoint
	if len(rows) == 0 {
		return out
	}

	type key struct {
		b, inbound int64
		email      string
	}
	acc := map[key]*dto.ClientTrafficPoint{}

	for _, r := range rows {
		b := r.TS / bucketSec * bucketSec
		k := key{b, int64(r.InboundID), r.Email}
		pt := acc[k]
		if pt == nil {
			pt = &dto.ClientTrafficPoint{TS: b, InboundID: r.InboundID, Email: r.Email}
			acc[k] = pt
		}
		pt.UpDelta += r.UpDelta
		pt.DownDelta += r.DownDelta
	}

	for _, pt := range acc {
		out = append(out, *pt)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TS != out[j].TS {
			return out[i].TS < out[j].TS
		}
		if out[i].InboundID != out[j].InboundID {
			return out[i].InboundID < out[j].InboundID
		}
		return out[i].Email < out[j].Email
	})
	return out
}

// --- Прогоны тестов ---

// TestRuns — GET /api/metrics/test-runs?range=&testerId=&subscription=
func (h *MetricsHandlers) TestRuns(w http.ResponseWriter, r *http.Request) {
	rg := parseMetricsRange(r.URL.Query().Get("range"))
	from := time.Unix(rg.from, 0).UTC()
	to := time.Unix(rg.to, 0).UTC()
	testerID := r.URL.Query().Get("testerId")
	subscription := r.URL.Query().Get("subscription")

	runs, err := h.db.TestRuns(from, to, testerID, subscription)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	names := h.testerNames()
	out := make([]dto.MetricsTestRun, 0, len(runs))
	for _, run := range runs {
		out = append(out, testRunToDTO(run, names[run.TesterID], nil))
	}
	respondJSON(w, http.StatusOK, dto.MetricsTestRunsResponse{Range: rg.key, Runs: out})
}

// TestRunDetail — GET /api/metrics/test-runs/{id} — прогон + per-key результаты.
func (h *MetricsHandlers) TestRunDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	run, ok, err := h.db.TestRunByID(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		respondError(w, http.StatusNotFound, "run not found")
		return
	}

	keys, err := h.db.TestKeyResults(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	keyDTOs := make([]dto.MetricsKeyResult, 0, len(keys))
	for _, k := range keys {
		kd := dto.MetricsKeyResult{
			ID:                k.ID,
			Label:             k.Label,
			Status:            k.Status,
			IP:                k.IP,
			LatencyMs:         k.LatencyMs,
			AvgSpeedKbps:      k.AvgSpeedKbps,
			StabilityPct:      k.StabilityPct,
			Reconnects:        k.Reconnects,
			TotalDownloadedMB: k.TotalDownloadedMB,
			SessionsOK:        k.SessionsOK,
			SessionsFail:      k.SessionsFail,
			DurationSec:       k.DurationSec,
			TestedAt:          k.TestedAt,
		}
		if k.KeyID != nil {
			kd.KeyID = *k.KeyID
		}
		keyDTOs = append(keyDTOs, kd)
	}

	names := h.testerNames()
	respondJSON(w, http.StatusOK, testRunToDTO(*run, names[run.TesterID], keyDTOs))
}

// testerNames возвращает карту tester_id → имя (одним запросом).
func (h *MetricsHandlers) testerNames() map[int64]string {
	out := map[int64]string{}
	testers, err := h.db.ListTesters()
	if err != nil {
		return out
	}
	for _, t := range testers {
		out[t.ID] = t.Name
	}
	return out
}

func testRunToDTO(run TestRun, testerName string, results []dto.MetricsKeyResult) dto.MetricsTestRun {
	d := dto.MetricsTestRun{
		ID:             run.ID,
		TesterID:       run.TesterID,
		TesterName:     testerName,
		SubscriptionID: run.SubscriptionID,
		Status:         run.Status,
		Total:          run.Total,
		OKCount:        run.OKCount,
		FailCount:      run.FailCount,
		StartedAt:      run.StartedAt,
		FinishedAt:     run.FinishedAt,
	}
	if run.Error != nil {
		d.Error = *run.Error
	}
	if results != nil {
		d.Results = results
	}
	return d
}

// --- Доступность панелей ---

// Availability — GET /api/metrics/availability — последний снапшот по панели.
func (h *MetricsHandlers) Availability(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.PanelAvailability()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]dto.MetricsAvailability, 0, len(rows))
	for _, a := range rows {
		out = append(out, dto.MetricsAvailability{
			PanelID:        a.PanelID,
			Name:           a.Name,
			LastSnapshotTS: a.LastSnapshotTS,
		})
	}
	respondJSON(w, http.StatusOK, out)
}
