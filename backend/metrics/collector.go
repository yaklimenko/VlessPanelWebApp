package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"vlesspanel/dto"
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

// PanelStore — минимальный доступ к реестру панелей и подписок, нужный
// коллектору. Реализация: *Storage (storage.go). Отделён интерфейсом, чтобы
// подпакет metrics не зависел от корневого backend (нет циклического импорта).
type PanelStore interface {
	LoadPanels() ([]model.Panel, error)
	ListSubscriptions() ([]model.Subscription, error)
}

// PanelAPI — клиент 3X-UI панели, нужный коллектору (инбаунды для трафика).
// Реализация: *PanelAPI (panelapi.go). Сужается до фактически используемых
// методов, чтобы подпакет metrics оставался standalone.
type PanelAPI interface {
	ListInbounds(panel model.Panel) ([]xui.XUIInbound, error)
}

// DaemonClient — клиент демона тестов (vlesssubtest), нужный коллектору.
// Реализация: *vlessSubTestClient (vlesssubtest.go).
type DaemonClient interface {
	Status() dto.VlessSubTestStatus
	ListRuns(from, to time.Time) ([]dto.DaemonRun, error)
}

// Параметры коллектора (Этап 1, решения из «Раздел статистики — задачи.md»).
const (
	collectInterval   = 5 * time.Minute    // опрос телеметрии панелей
	historyBucket     = 60                 // ⚠️ реально работают только bucket 2/30/60
	telemetryWindow   = 5 * time.Minute    // окно агрегации снапшота
	backfillRunsSince = 7 * 24 * time.Hour // стартовый забор прогонов
)

// snapshotWindowSec — длина окна снапшота в секундах (5 минут). Точки истории
// группируются по бакетам этой длины: bucketTS = t / snapshotWindowSec * snapshotWindowSec.
const snapshotWindowSec = int64(telemetryWindow / time.Second)

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
	storage   PanelStore
	panelsAPI PanelAPI
	telemetry TelemetryClient
	daemon    DaemonClient
	daemonURL string // базовый URL демона (сверка с base_url тестера в БД)
	log       *log.Logger

	// Собственность goroutine телеметрии.
	telemetryMu sync.Mutex // TryLock — не накладываем циклы друг на друга
	lastTS      map[string]int64
	baselines   map[string]map[string]clientCounters

	// Собственность goroutine прогонов.
	runsMu   sync.Mutex
	lastRuns time.Time

	// TG-алерты (Этап 2): проверка порогов после каждого цикла сбора.
	// nil — алерты выключены (нет токена/chat_id).
	alerts *AlertManager
}

type clientCounters struct{ up, down int64 }

// NewMetricsCollector создаёт коллектор. daemonURL — конфигурируемый адрес
// демона (VLESSPANEL_VLESSSUBTEST_DAEMON_URL), по нему сверяем testers.base_url.
func NewMetricsCollector(db *MetricsDB, storage PanelStore, panelsAPI PanelAPI,
	telemetry TelemetryClient, daemon DaemonClient, daemonURL string, logger *log.Logger) *MetricsCollector {
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

// SetAlerts подключает менеджер TG-алертов (nil — алерты выключены).
// Вызывается из main после создания коллектора (поле неэкспортируемое).
func (c *MetricsCollector) SetAlerts(a *AlertManager) { c.alerts = a }

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

	// Лёгкий контакт с демоном(ами) каждый цикл, чтобы last_heartbeat_at был
	// свежим и алерт «тестер не отвечает» (дефолт 15 мин) был честным:
	// полный контакт (забор прогонов) — раз в 6 часов, этого слишком редко.
	c.touchTesterHeartbeats(now)
	if c.alerts != nil {
		c.alerts.CheckTesters()
	}
}

// touchTesterHeartbeats — Status() демона раз в цикл сбора (5 мин) для
// наших тестеров (base_url == daemonURL); успех продлевает heartbeat.
// Ошибка только логируется — недоступность видит CheckTesters по протухшему
// last_heartbeat_at.
func (c *MetricsCollector) touchTesterHeartbeats(now time.Time) {
	testers, err := c.db.ListTesters()
	if err != nil {
		c.log.Printf("metrics: testers (heartbeat): %v", err)
		return
	}
	for _, t := range testers {
		if t.Enabled == 0 {
			continue
		}
		if strings.TrimRight(t.BaseURL, "/") != c.daemonURL {
			continue // не наш демон — пропускаем (как в collectRuns)
		}
		st := c.daemon.Status()
		if !st.Available {
			c.log.Printf("metrics: демон %s не отвечает (heartbeat)", t.Name)
			continue
		}
		if err := c.db.TouchTesterHeartbeat(t.ID, now); err != nil {
			c.log.Printf("metrics: heartbeat тестера %s: %v", t.Name, err)
		}
	}
}

// panelDown — сигнал «панель не отдала телеметрию» в менеджер алертов.
func (c *MetricsCollector) panelDown(p model.Panel, now time.Time) {
	if c.alerts != nil {
		c.alerts.CheckPanelDown(p)
	}
}

// collectPanel собирает одну панель: status → history (параллельно) → inbounds.
// Любая ошибка = панель не отдаёт телеметрию: пишем лог и выходим, ничего не
// сохраняя (пропуск данных — сигнал недоступности). Повторных опросов нет.
func (c *MetricsCollector) collectPanel(p model.Panel, now time.Time) {
	st, err := c.telemetry.ServerStatus(p)
	if err != nil {
		c.log.Printf("metrics: панель %s недоступна (server/status): %v", p.ID, err)
		c.panelDown(p, now)
		return
	}

	points, err := c.fetchHistory(p)
	if err != nil {
		c.log.Printf("metrics: панель %s недоступна (server/history): %v", p.ID, err)
		c.panelDown(p, now)
		return
	}

	rec, wrote, err := c.writeSnapshots(p, st, points, now)
	if err != nil {
		c.log.Printf("metrics: запись снапшотов %s: %v", p.ID, err)
		c.panelDown(p, now)
	} else if wrote {
		// Свежий снапшот записан — проверяем пороги (и снимаем panel_down).
		if c.alerts != nil {
			c.alerts.CheckPanel(p, rec)
		}
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
// контакте (нет ни памяти, ни строк в БД) — 6 часов назад: 3X-UI хранит
// ~6 часов истории, забираем её ВСЮ и разбиваем на 5-минутные окна, чтобы
// график с самого старта был полным, а не из одного окна. После рестарта —
// максимум строки БД (инкрементальный догон пропущенного), он же перекрывает
// 6-часовую глубину (строки БД всегда свежее).
func (c *MetricsCollector) lastSeen(panelID string, windowStart int64) int64 {
	if ts, ok := c.lastTS[panelID]; ok {
		return ts
	}
	ts := windowStart - 6*3600
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

// writeSnapshots агрегирует новые точки истории (t > lastSeen) по 5-минутным
// бакетам (bucketTS = t / 300 * 300) и пишет по строке на каждый бакет — так
// при первом контакте забирается вся 6-часовая история панели (~72 окна), а не
// только текущее. Бакеты пишутся от старых к новым; INSERT OR REPLACE по
// UNIQUE(panel_id, ts) делает повторную запись бакета идемпотентной (пришла
// новая точка в уже записанное окно — строка перезаписывается более полной
// агрегацией, дублей нет). Поля, которых нет в истории (swap/disk/open_conns/
// xray_ok + кумулятивные netTraffic), — только в самом свежем бакете
// (server/status — это текущий момент), у старых — nil/дефолт (xray_ok=1).
// lastSeen продвигается до максимального ts точки только при успешной записи.
func (c *MetricsCollector) writeSnapshots(p model.Panel, st *xui.ServerStatus, points map[string][]xui.HistoryPoint, now time.Time) (SnapshotRecord, bool, error) {
	windowStart := now.UTC().Truncate(telemetryWindow).Unix()
	lastSeen := c.lastSeen(p.ID, windowStart)

	// Бакет с точками истории (windowAgg на каждое 5-минутное окно).
	type bucket struct {
		agg  *windowAgg
		maxT int64
	}
	buckets := map[int64]*bucket{}
	var maxT int64
	for metric, pts := range points {
		for _, pt := range pts {
			if pt.T <= lastSeen {
				continue
			}
			bts := pt.T / snapshotWindowSec * snapshotWindowSec
			b := buckets[bts]
			if b == nil {
				b = &bucket{agg: &windowAgg{}}
				buckets[bts] = b
			}
			b.agg.add(metric, pt.V)
			if pt.T > b.maxT {
				b.maxT = pt.T
			}
			if pt.T > maxT {
				maxT = pt.T
			}
		}
	}

	if maxT == 0 {
		return SnapshotRecord{}, false, nil // новых точек нет — не пишем пустые окна
	}

	// Сортируем бакеты по возрастанию ts: старые окна пишем раньше новых.
	tsList := make([]int64, 0, len(buckets))
	for bts := range buckets {
		tsList = append(tsList, bts)
	}
	sort.Slice(tsList, func(i, j int) bool { return tsList[i] < tsList[j] })

	xrayOK := 1
	if st.Xray.State != "" && st.Xray.State != "running" {
		xrayOK = 0
	}

	freshTS := tsList[len(tsList)-1] // ближайший к now бакет — статус панели актуален только для него
	var lastRec SnapshotRecord
	wrote := false
	for _, bts := range tsList {
		b := buckets[bts]
		rec := SnapshotRecord{
			PanelID:   p.ID,
			TS:        bts,
			CPUAvg:    avg(b.agg.cpuSum, b.agg.cpuN),
			CPUMax:    maxOrNil(b.agg.cpuMax, b.agg.cpuN),
			MemAvg:    avg(b.agg.memSum, b.agg.memN),
			MemMax:    maxOrNil(b.agg.memMax, b.agg.memN),
			Load1Avg:  avg(b.agg.load1Sum, b.agg.load1N),
			Load5Avg:  avg(b.agg.load5Sum, b.agg.load5N),
			Load15Avg: avg(b.agg.load15Sum, b.agg.load15N),
			NetUp:     int64Ptr(b.agg.netUpSum * historyBucket),
			NetDown:   int64Ptr(b.agg.netDownSum * historyBucket),
			OnlineAvg: roundIntPtr(avgOrZero(b.agg.onlineSum, b.agg.onlineN), b.agg.onlineN),
			OnlineMax: roundIntPtr(b.agg.onlineMax, b.agg.onlineN),
			XrayOK:    1,
		}
		if bts == freshTS {
			// server/status — текущий момент, поэтому только в самом свежем окне.
			rec.SwapAvg = percentOf(st.Swap.Current, st.Swap.Total)
			rec.NetTrafficSent = int64Ptr(st.NetIO.Up)
			rec.NetTrafficRecv = int64Ptr(st.NetIO.Down)
			rec.DiskUsed = int64Ptr(st.Disk.Current)
			rec.DiskTotal = int64Ptr(st.Disk.Total)
			rec.OpenConnsMax = intPtr(st.TCPCount)
			rec.XrayOK = xrayOK
		}
		if err := c.db.InsertSnapshot(rec); err != nil {
			return SnapshotRecord{}, false, err
		}
		lastRec = rec
		wrote = true
	}

	// Продвигаем lastSeen только после успешной записи.
	c.lastTS[p.ID] = maxT
	return lastRec, wrote, nil
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
func (c *MetricsCollector) insertRun(testerID int64, run dto.DaemonRun, subsKeys map[string][]model.SubKey) (bool, error) {
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

	var keys []dto.DaemonKeyResult
	if len(run.Results) > 0 {
		if err := json.Unmarshal(run.Results, &keys); err != nil {
			c.log.Printf("metrics: прогон %s: разбор per-key результатов: %v", run.ID, err)
		}
	}

	keyResults := make([]TestKeyResult, 0, len(keys))
	for _, k := range keys {
		keyResults = append(keyResults, TestKeyResult{
			KeyID:             matchSubKeyID(subsKeys[subID], k.Remark),
			Label:             k.Remark,
			Status:            normalizeKeyStatus(k.Status),
			IP:                k.IP,
			AvgSpeedKbps:      k.AvgSpeedKbps,
			StabilityPct:      k.StabilityPct,
			Reconnects:        k.Reconnects,
			TotalDownloadedMB: k.TotalDownloadedMB,
			SessionsOK:        k.SessionsOK,
			SessionsFail:      k.SessionsFail,
			DurationSec:       k.DurationSec,
			LatencyMs:         floatToIntPtr(k.LatencyMs),
			TestedAt:          run.StartedAt.UTC().Format(runTimeFormat),
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

// floatToIntPtr округляет float (latency_ms демона) до int и возвращает
// указатель; для нулевых/отрицательных значений возвращает nil (нет данных).
func floatToIntPtr(v float64) *int {
	if v <= 0 {
		return nil
	}
	i := int(math.Round(v))
	return &i
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
