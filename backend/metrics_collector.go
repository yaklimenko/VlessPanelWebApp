package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"vlesspanel/model"
	"vlesspanel/xui"
)

// TelemetryClient — клиент телеметрии панели (server/status + server/history).
// Реализация: *PanelAPI (panelapi_telemetry.go). Отделён интерфейсом, чтобы
// коллектор можно было юнит-тестировать без реальных панелей.
type TelemetryClient interface {
	ServerStatus(panel model.Panel) (*xui.ServerStatus, error)
	ServerHistory(panel model.Panel, metric string, bucket int) ([]xui.HistoryPoint, error)
}

// Параметры коллектора (Этап 1, решения из «Раздел статистики — задачи.md»).
const (
	collectInterval   = 5 * time.Minute    // опрос телеметрии панелей
	historyBucket     = 60                 // ⚠️ реально работают только bucket 2/30/60
	telemetryWindow   = 5 * time.Minute    // окно агрегации снапшота
	backfillRunsSince = 7 * 24 * time.Hour // стартовый забор прогонов
)

// historyMetrics — метрики, которые панель держит в истории (avg+max считаем САМИ).
// swap/disk/open_conns/xray_ok в истории нет — берём из server/status.
var historyMetrics = []string{"cpu", "mem", "netUp", "netDown", "online", "load1", "load5", "load15"}

// MetricsCollector — горутины сбора телеметрии панелей (5 мин) и результатов
// прогонов тестов (каждые 6 часов со сдвигом на 15 минут) в metrics.db.
//
// Свойства (по заметке):
//   - забор инкрементальный: помним last_ts по каждой панели, берём только
//     новые точки истории, ничего не перезабираем;
//   - панель, не отдавшая телеметрию, НЕ опрашивается повторно в цикле —
//     только лог (пропуск данных сам по себе сигнал), панели не долбим;
//   - клиентский трафик — только дельты с прошлого среза, пустые не пишем.
type MetricsCollector struct {
	db        *MetricsDB
	storage   *Storage
	panelsAPI PanelClient
	telemetry TelemetryClient
	daemon    VlessSubTestClient
	daemonURL string // базовый URL демона (сверка с base_url тестера в БД)
	log       *log.Logger

	// Собственность goroutine телеметрии.
	telemetryMu sync.Mutex // TryLock — не накладываем циклы друг на друга
	lastTS      map[string]int64
	baselines   map[string]map[string]clientCounters

	// Собственность goroutine прогонов.
	runsMu   sync.Mutex
	lastRuns time.Time
}

type clientCounters struct{ up, down int64 }

// NewMetricsCollector создаёт коллектор. daemonURL — конфигурируемый адрес
// демона (VLESSPANEL_VLESSSUBTEST_DAEMON_URL), по нему сверяем testers.base_url.
func NewMetricsCollector(db *MetricsDB, storage *Storage, panelsAPI PanelClient,
	telemetry TelemetryClient, daemon VlessSubTestClient, daemonURL string, logger *log.Logger) *MetricsCollector {
	if logger == nil {
		logger = log.Default()
	}
	return &MetricsCollector{
		db:        db,
		storage:   storage,
		panelsAPI: panelsAPI,
		telemetry: telemetry,
		daemon:    daemon,
		daemonURL: strings.TrimRight(daemonURL, "/"),
		log:       logger,
		lastTS:    map[string]int64{},
		baselines: map[string]map[string]clientCounters{},
		lastRuns:  time.Now().UTC().Add(-backfillRunsSince),
	}
}

// Start запускает три горутины: телеметрия (5 мин), прогоны (6ч :15),
// retention (сразу + раз в сутки). Останавливаются по ctx.
func (c *MetricsCollector) Start(ctx context.Context) {
	go c.telemetryLoop(ctx)
	go c.runsLoop(ctx)
	go c.retentionLoop(ctx)
}

// --- Телеметрия панелей ---

func (c *MetricsCollector) telemetryLoop(ctx context.Context) {
	c.collectTelemetry(time.Now())
	ticker := time.NewTicker(collectInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			c.collectTelemetry(now)
		}
	}
}

// collectTelemetry опрашивает все панели из panels.json. Если предыдущий цикл
// ещё идёт (панель зависла) — пропускаем такт, чтобы не наслаиваться.
func (c *MetricsCollector) collectTelemetry(now time.Time) {
	if !c.telemetryMu.TryLock() {
		c.log.Printf("metrics: предыдущий цикл телеметрии ещё идёт, пропускаю такт")
		return
	}
	defer c.telemetryMu.Unlock()

	panels, err := c.storage.LoadPanels()
	if err != nil {
		c.log.Printf("metrics: load panels: %v", err)
		return
	}
	if err := c.db.SyncPanels(panels); err != nil {
		c.log.Printf("metrics: sync panels mirror: %v", err)
	}

	for _, p := range panels {
		c.collectPanel(p, now)
	}
}

// collectPanel собирает одну панель: status → history (параллельно) → inbounds.
// Любая ошибка = панель не отдаёт телеметрию: пишем лог и выходим, ничего не
// сохраняя (пропуск данных — сигнал недоступности). Повторных опросов нет.
func (c *MetricsCollector) collectPanel(p model.Panel, now time.Time) {
	st, err := c.telemetry.ServerStatus(p)
	if err != nil {
		c.log.Printf("metrics: панель %s недоступна (server/status): %v", p.ID, err)
		return
	}

	points, err := c.fetchHistory(p)
	if err != nil {
		c.log.Printf("metrics: панель %s недоступна (server/history): %v", p.ID, err)
		return
	}

	if err := c.writeSnapshots(p, st, points, now); err != nil {
		c.log.Printf("metrics: запись снапшотов %s: %v", p.ID, err)
	}

	inbounds, err := c.panelsAPI.ListInbounds(p)
	if err != nil {
		c.log.Printf("metrics: панель %s недоступна (inbounds/list): %v", p.ID, err)
		return
	}
	if err := c.writeTraffic(p, inbounds, now); err != nil {
		c.log.Printf("metrics: запись трафика %s: %v", p.ID, err)
	}
}

// fetchHistory тянет историю всех метрик параллельно (bucket=60) и режет её
// по инкрементальному lastSeen. Возвращает точки старше lastSeen.
func (c *MetricsCollector) fetchHistory(p model.Panel) (map[string][]xui.HistoryPoint, error) {
	type res struct {
		metric string
		pts    []xui.HistoryPoint
		err    error
	}
	ch := make(chan res, len(historyMetrics))
	for _, m := range historyMetrics {
		go func(m string) {
			pts, err := c.telemetry.ServerHistory(p, m, historyBucket)
			ch <- res{metric: m, pts: pts, err: err}
		}(m)
	}

	out := make(map[string][]xui.HistoryPoint, len(historyMetrics))
	for range historyMetrics {
		r := <-ch
		if r.err != nil {
			return nil, fmt.Errorf("%s: %w", r.metric, r.err)
		}
		out[r.metric] = r.pts
	}
	return out, nil
}

// lastSeen возвращает последний потреблённый ts истории панели. При первом
// контакте (нет ни памяти, ни строк в БД) — начало текущего окна минус 1 сек,
// чтобы взять точки текущего окна (t >= windowStart), но не схлопнуть в одну
// строку всю 6-часовую историю панели. После рестарта — максимум строки БД
// (инкрементальный догон пропущенного).
func (c *MetricsCollector) lastSeen(panelID string, windowStart int64) int64 {
	if ts, ok := c.lastTS[panelID]; ok {
		return ts
	}
	ts := windowStart - 1
	if maxTS, ok := c.db.MaxSnapshotTS(panelID); ok && maxTS > ts {
		ts = maxTS
	}
	c.lastTS[panelID] = ts
	return ts
}

// windowAgg — агрегатор точек истории за цикл сбора (одно 5-минутное окно).
type windowAgg struct {
	cpuSum, cpuMax float64
	cpuN           int
	memSum, memMax float64
	memN           int

	load1Sum, load1Max float64
	load1N             int
	load5Sum, load5Max float64
	load5N             int
	load15Sum          float64
	load15N            int

	netUpSum, netDownSum int64

	onlineSum, onlineMax float64
	onlineN              int
}

func (w *windowAgg) add(metric string, v float64) {
	switch metric {
	case "cpu":
		w.cpuSum += v
		if v > w.cpuMax {
			w.cpuMax = v
		}
		w.cpuN++
	case "mem":
		w.memSum += v
		if v > w.memMax {
			w.memMax = v
		}
		w.memN++
	case "netUp":
		w.netUpSum += int64(math.Round(v))
	case "netDown":
		w.netDownSum += int64(math.Round(v))
	case "online":
		w.onlineSum += v
		if v > w.onlineMax {
			w.onlineMax = v
		}
		w.onlineN++
	case "load1":
		w.load1Sum += v
		if v > w.load1Max {
			w.load1Max = v
		}
		w.load1N++
	case "load5":
		w.load5Sum += v
		if v > w.load5Max {
			w.load5Max = v
		}
		w.load5N++
	case "load15":
		w.load15Sum += v
		w.load15N++
	}
}

func avg(sum float64, n int) *float64 {
	if n == 0 {
		return nil
	}
	v := sum / float64(n)
	return &v
}

func maxOrNil(v float64, n int) *float64 {
	if n == 0 {
		return nil
	}
	return &v
}

func intPtr(v int) *int { return &v }

func int64Ptr(v int64) *int64 { return &v }

// writeSnapshots агрегирует все новые точки истории (t > lastSeen) в ОДНО
// окно — начало окна сбора (row.ts = floor(now/5мин)). Так каждый цикл даёт
// ровно одну строку на панель, строки не затирают друг друга (точки хвоста
// прошлого окна приписываются текущему — за 5 минут их ровно ~5 штук).
// Поля, которых нет в истории (swap/disk/open_conns/xray_ok + кумулятивные
// netTraffic) — из server/status. lastSeen продвигается только при записи.
func (c *MetricsCollector) writeSnapshots(p model.Panel, st *xui.ServerStatus, points map[string][]xui.HistoryPoint, now time.Time) error {
	windowStart := now.UTC().Truncate(telemetryWindow).Unix()
	lastSeen := c.lastSeen(p.ID, windowStart)

	agg := &windowAgg{}
	var maxT int64
	for metric, pts := range points {
		for _, pt := range pts {
			if pt.T <= lastSeen {
				continue
			}
			agg.add(metric, pt.V)
			if pt.T > maxT {
				maxT = pt.T
			}
		}
	}

	if maxT == 0 {
		return nil // новых точек нет — не пишем пустые окна
	}

	xrayOK := 1
	if st.Xray.State != "" && st.Xray.State != "running" {
		xrayOK = 0
	}

	rec := SnapshotRecord{
		PanelID:        p.ID,
		TS:             windowStart,
		CPUAvg:         avg(agg.cpuSum, agg.cpuN),
		CPUMax:         maxOrNil(agg.cpuMax, agg.cpuN),
		MemAvg:         avg(agg.memSum, agg.memN),
		MemMax:         maxOrNil(agg.memMax, agg.memN),
		SwapAvg:        percentOf(st.Swap.Current, st.Swap.Total),
		Load1Avg:       avg(agg.load1Sum, agg.load1N),
		Load5Avg:       avg(agg.load5Sum, agg.load5N),
		Load15Avg:      avg(agg.load15Sum, agg.load15N),
		NetUp:          int64Ptr(agg.netUpSum),
		NetDown:        int64Ptr(agg.netDownSum),
		NetTrafficSent: int64Ptr(st.NetIO.Up),
		NetTrafficRecv: int64Ptr(st.NetIO.Down),
		DiskUsed:       int64Ptr(st.Disk.Current),
		DiskTotal:      int64Ptr(st.Disk.Total),
		OnlineAvg:      roundIntPtr(avgOrZero(agg.onlineSum, agg.onlineN), agg.onlineN),
		OnlineMax:      roundIntPtr(agg.onlineMax, agg.onlineN),
		OpenConnsMax:   intPtr(st.TCPCount),
		XrayOK:         xrayOK,
	}
	if err := c.db.InsertSnapshot(rec); err != nil {
		return err
	}

	// Продвигаем lastSeen только после успешной записи.
	c.lastTS[p.ID] = maxT
	return nil
}

// roundIntPtr округляет v до int; при n==0 возвращает nil.
func roundIntPtr(v float64, n int) *int {
	if n == 0 {
		return nil
	}
	return intPtr(int(math.Round(v)))
}

func avgOrZero(sum float64, n int) float64 {
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func percentOf(current, total int64) *float64 {
	if total <= 0 {
		return nil
	}
	v := float64(current) / float64(total) * 100
	return &v
}

// writeTraffic пишет срезы счётчиков инбаундов и дельты клиентов.
// Клиентские дельты считаются от базлайна прошлого среза (в памяти); пустые
// клиенты (0/0) не пишутся; сброс счётчика (отрицательная дельта) — 0.
func (c *MetricsCollector) writeTraffic(p model.Panel, inbounds []xui.XUIInbound, now time.Time) error {
	ts := now.UTC().Truncate(telemetryWindow).Unix()

	prev, hadBaseline := c.baselines[p.ID]
	seen := make(map[string]clientCounters, 64)

	for _, ib := range inbounds {
		rec := InboundTrafficRecord{
			PanelID:   p.ID,
			InboundID: ib.ID,
			Remark:    ib.Remark,
			TS:        ts,
			Up:        ib.Up,
			Down:      ib.Down,
			Total:     ib.Total,
		}
		if err := c.db.InsertInboundTraffic(rec); err != nil {
			return err
		}

		for _, cs := range ib.ClientStats {
			if cs.Email == "" {
				continue
			}
			key := fmt.Sprintf("%d:%s", ib.ID, cs.Email)
			cur := clientCounters{up: cs.Up, down: cs.Down}
			seen[key] = cur

			if !hadBaseline {
				continue // первый срез — только базлайн
			}
			base, ok := prev[key]
			if !ok {
				continue // новый клиент — базлайн с этого среза
			}
			upDelta := cur.up - base.up
			downDelta := cur.down - base.down
			if upDelta < 0 {
				upDelta = 0
			}
			if downDelta < 0 {
				downDelta = 0
			}
			if upDelta == 0 && downDelta == 0 {
				continue // пустые клиенты не мусорят
			}
			if err := c.db.InsertClientTraffic(ClientTrafficRecord{
				PanelID:   p.ID,
				InboundID: ib.ID,
				Email:     cs.Email,
				TS:        ts,
				UpDelta:   upDelta,
				DownDelta: downDelta,
			}); err != nil {
				return err
			}
		}
	}

	// Обрезаем базлайны исчезнувших клиентов (память не течёт).
	c.baselines[p.ID] = seen
	return nil
}

// --- Прогоны тестов (doom-scroll) ---

func (c *MetricsCollector) runsLoop(ctx context.Context) {
	c.collectRuns(time.Now())
	for {
		next := nextRunsTick(time.Now().UTC())
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			c.collectRuns(time.Now())
		}
	}
}

// nextRunsTick — ближайший момент сбора прогонов: 00:15/06:15/12:15/18:15 UTC.
// Первый опрос не в полночь, а в 00:15 (решение из задач).
func nextRunsTick(now time.Time) time.Time {
	h := (now.Hour() / 6) * 6
	cand := time.Date(now.Year(), now.Month(), now.Day(), h, 15, 0, 0, time.UTC)
	if !cand.After(now) {
		cand = cand.Add(6 * time.Hour)
	}
	return cand
}

// collectRuns забирает результаты прогонов с демона(ов) за [lastRuns, now] и
// пишет в test_runs + test_key_results (дедупликация внутри InsertTestRun).
func (c *MetricsCollector) collectRuns(now time.Time) {
	if !c.runsMu.TryLock() {
		c.log.Printf("metrics: предыдущий забор прогонов ещё идёт, пропускаю")
		return
	}
	defer c.runsMu.Unlock()

	now = now.UTC()
	testers, err := c.db.ListTesters()
	if err != nil {
		c.log.Printf("metrics: testers: %v", err)
		return
	}

	// Карта подписок (name → ключи) для best-effort связки key_id красных ключей.
	subsKeys := map[string][]model.SubKey{}
	if subs, err := c.storage.ListSubscriptions(); err == nil {
		for _, s := range subs {
			subsKeys[s.Name] = s.Keys
		}
	}

	anyOK := false
	for _, t := range testers {
		if t.Enabled == 0 {
			continue
		}
		if strings.TrimRight(t.BaseURL, "/") != c.daemonURL {
			c.log.Printf("metrics: тестер %s (%s) — не наш демон (ожидался %s), пропускаю",
				t.Name, t.BaseURL, c.daemonURL)
			continue
		}

		runs, err := c.daemon.ListRuns(c.lastRuns, now)
		if err != nil {
			c.log.Printf("metrics: забор прогонов с %s: %v", t.Name, err)
			continue
		}
		anyOK = true
		if err := c.db.TouchTesterHeartbeat(t.ID, now); err != nil {
			c.log.Printf("metrics: heartbeat тестера %s: %v", t.Name, err)
		}

		inserted := 0
		for _, run := range runs {
			ok, err := c.insertRun(t.ID, run, subsKeys)
			if err != nil {
				c.log.Printf("metrics: сохранение прогона %s: %v", run.ID, err)
				continue
			}
			if ok {
				inserted++
			}
		}
		if len(runs) > 0 {
			c.log.Printf("metrics: тестер %s: %d прогонов, новых %d", t.Name, len(runs), inserted)
		}
	}
	// Курсор двигаем только при успешном заборе: если демон лежал, окно
	// не пропадает — следующий тик заберёт его целиком.
	if anyOK {
		c.lastRuns = now
	}
}

// insertRun маппит прогон демона в test_runs + test_key_results.
func (c *MetricsCollector) insertRun(testerID int64, run DaemonRun, subsKeys map[string][]model.SubKey) (bool, error) {
	subID := subscriptionNameFromURL(run.SubscriptionURL)

	status := "ok"
	if run.Error != "" {
		status = "failed"
	} else if run.Failed > 0 || run.Degraded > 0 {
		status = "partial"
	}

	var runErr *string
	if run.Error != "" {
		e := run.Error
		runErr = &e
	}

	rec := &TestRun{
		TesterID:       testerID,
		SubscriptionID: subID,
		Status:         status,
		Total:          run.Total,
		OKCount:        run.OK,
		FailCount:      run.Failed + run.Degraded,
		Error:          runErr,
		StartedAt:      run.StartedAt.UTC().Format(runTimeFormat),
		FinishedAt:     run.FinishedAt.UTC().Format(runTimeFormat),
	}

	var keys []DaemonKeyResult
	if len(run.Results) > 0 {
		if err := json.Unmarshal(run.Results, &keys); err != nil {
			c.log.Printf("metrics: прогон %s: разбор per-key результатов: %v", run.ID, err)
		}
	}

	keyResults := make([]TestKeyResult, 0, len(keys))
	for _, k := range keys {
		keyResults = append(keyResults, TestKeyResult{
			KeyID:     matchSubKeyID(subsKeys[subID], k.Remark),
			Label:     k.Remark,
			Status:    normalizeKeyStatus(k.Status),
			IP:        k.IP,
			YouTube:   k.Youtube,
			Instagram: k.Instagram,
			TestedAt:  run.StartedAt.UTC().Format(runTimeFormat),
		})
	}

	_, inserted, err := c.db.InsertTestRun(rec, keyResults)
	return inserted, err
}

// subscriptionNameFromURL достаёт имя подписки из subscription_url демона
// (https://<host>/sub/<Name> → <Name>, URL-декодированное). Если последний
// сегмент — служебный "sub" или пустой, возвращаем URL как есть.
func subscriptionNameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	seg := path.Base(u.Path)
	if seg == "." || seg == "/" || seg == "" || seg == "sub" {
		return raw
	}
	if dec, err := url.PathUnescape(seg); err == nil {
		seg = dec
	}
	return seg
}

// matchSubKeyID ищет SubKey, чей vless-фрагмент (#remark) совпадает с remark
// результата демона — так красный ключ связывается с подпиской (key_id =
// id SubKey из VlessPanel, тот что убирается через removeKeyId).
func matchSubKeyID(keys []model.SubKey, remark string) *string {
	if remark == "" {
		return nil
	}
	for _, k := range keys {
		if u, err := url.Parse(k.Link); err == nil && u.Fragment == remark {
			id := k.ID
			return &id
		}
	}
	return nil
}

// normalizeKeyStatus приводит статус ключа демона к словарю схемы
// (OK|FAIL|TIMEOUT|ERROR).
func normalizeKeyStatus(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "OK":
		return "OK"
	case "FAILED", "FAIL":
		return "FAIL"
	case "TIMEOUT":
		return "TIMEOUT"
	case "":
		return "ERROR"
	default:
		return strings.ToUpper(s)
	}
}

// --- Retention ---

func (c *MetricsCollector) retentionLoop(ctx context.Context) {
	if err := c.db.CleanupRetention(time.Now()); err != nil {
		c.log.Printf("metrics: retention при старте: %v", err)
	}
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.db.CleanupRetention(time.Now()); err != nil {
				c.log.Printf("metrics: retention: %v", err)
			}
		}
	}
}
