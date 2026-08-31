package metrics

import (
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"vlesspanel/model"
)

// TG-алерты (Этап 2 раздела статистики).
//
// Проверка порогов выполняется при каждом цикле сбора коллектора (5 мин):
//   - RAM > порога (берём mem_max — max «для алертов» по решению 29.08.2026);
//   - load1 > числа ядер панели × фактор (ядра — env, по умолчанию 1);
//   - аномальный рост трафика: net_up/net_down текущего окна > множителя
//     от среднего за окно N часов (мало истории — алерт не шлём);
//   - «тестер не отвечает»: last_heartbeat_at старше N минут (по таблице testers);
//   - недоступность панели: однократно при переходе в недоступно (дедуп покроет).
//
// Дедупликация — по ключу (panel_id + тип алерта) в таблице alert_states:
// повторный алерт не чаще раза в cooldown; при возврате в норму — OK-сообщение,
// тоже не чаще cooldown. Отправка в Telegram через Bot API sendMessage
// (parse_mode=HTML, экранирование html.EscapeString). Токен/чат — из env
// (VLESSPANEL_TG_BOT_TOKEN / VLESSPANEL_TG_CHAT_ID), НЕ хардкодить и НЕ коммитить.
// Отправка не блокирует коллектор: таймаут (дефолт 10с), ошибки только в лог.

// Типы алертов (составной ключ дедупа: "<тип>:<panel_id|имя тестера>").
const (
	alertRAM        = "ram_high"
	alertLoad       = "load_high"
	alertTraffic    = "traffic_spike"
	alertPanelDown  = "panel_down"
	alertTesterDead = "tester_stale"
)

// AlertConfig — конфигурация TG-алертов (env с дефолтами, паттерн config.go).
type AlertConfig struct {
	Enabled           bool          // итоговый флаг: false если токен/чат не заданы
	BotToken          string        // VLESSPANEL_TG_BOT_TOKEN
	ChatID            string        // VLESSPANEL_TG_CHAT_ID
	RAMThresholdPct   float64       // VLESSPANEL_ALERT_RAM_PCT, дефолт 85 (% занятой RAM)
	LoadCores         float64       // VLESSPANEL_ALERT_LOAD_CORES, дефолт 1 (ядер на панелях)
	LoadFactor        float64       // VLESSPANEL_ALERT_LOAD_FACTOR, дефолт 1.0 (порог = ядра × фактор)
	TrafficMultiplier float64       // VLESSPANEL_ALERT_TRAFFIC_MULT, дефолт 5 (× среднего)
	TrafficWindow     time.Duration // VLESSPANEL_ALERT_TRAFFIC_WINDOW, дефолт 24h (базлайн)
	TrafficMinSamples int           // VLESSPANEL_ALERT_TRAFFIC_MIN_SAMPLES, дефолт 24 (мин. окон в базлайне)
	TrafficMinMbs     float64       // VLESSPANEL_ALERT_TRAFFIC_MIN_MBS, дефолт 2 (абс. минимум MB/s)
	StaleTesterAfter  time.Duration // VLESSPANEL_ALERT_TESTER_STALE, дефолт 15m (сердцебиение тестера)
	Cooldown          time.Duration // VLESSPANEL_ALERT_COOLDOWN, дефолт 6h (повторный алерт/OK)
	SendTimeout       time.Duration // VLESSPANEL_ALERT_SEND_TIMEOUT, дефолт 10s
}

// LoadAlertConfig читает конфигурацию алертов из env. Если токен или chat_id
// не заданы — алерты считаются выключенными (Enabled=false).
func LoadAlertConfig() AlertConfig {
	cfg := AlertConfig{
		Enabled:           envBool("VLESSPANEL_ALERTS_ENABLED", true),
		BotToken:          os.Getenv("VLESSPANEL_TG_BOT_TOKEN"),
		ChatID:            os.Getenv("VLESSPANEL_TG_CHAT_ID"),
		RAMThresholdPct:   envFloat("VLESSPANEL_ALERT_RAM_PCT", 85),
		LoadCores:         envFloat("VLESSPANEL_ALERT_LOAD_CORES", 1),
		LoadFactor:        envFloat("VLESSPANEL_ALERT_LOAD_FACTOR", 1.0),
		TrafficMultiplier: envFloat("VLESSPANEL_ALERT_TRAFFIC_MULT", 5),
		TrafficWindow:     envDuration("VLESSPANEL_ALERT_TRAFFIC_WINDOW", 24*time.Hour),
		TrafficMinSamples: envInt("VLESSPANEL_ALERT_TRAFFIC_MIN_SAMPLES", 24),
		TrafficMinMbs:     envFloat("VLESSPANEL_ALERT_TRAFFIC_MIN_MBS", 2),
		StaleTesterAfter:  envDuration("VLESSPANEL_ALERT_TESTER_STALE", 15*time.Minute),
		Cooldown:          envDuration("VLESSPANEL_ALERT_COOLDOWN", 12*time.Hour),
		SendTimeout:       envDuration("VLESSPANEL_ALERT_SEND_TIMEOUT", 10*time.Second),
	}
	if cfg.BotToken == "" || cfg.ChatID == "" {
		cfg.Enabled = false
	}
	return cfg
}

// tgSender — отправка сообщения в Telegram (мокается в тестах).
type tgSender interface {
	SendMessage(text string) error
}

// tgClient — реальный отправитель: Bot API sendMessage с parse_mode=HTML.
type tgClient struct {
	botToken string
	chatID   string
	client   *http.Client
	log      *log.Logger
}

// NewTGClient создаёт отправителя Telegram (таймаут на запрос — из конфига).
func NewTGClient(token, chatID string, timeout time.Duration, logger *log.Logger) *tgClient {
	if logger == nil {
		logger = log.Default()
	}
	return &tgClient{
		botToken: token,
		chatID:   chatID,
		client:   &http.Client{Timeout: timeout},
		log:      logger,
	}
}

// SendMessage отправляет текст в чат. Ошибки возвращаются — вызывающий
// (AlertManager) только логирует их, коллектор не роняется.
func (t *tgClient) SendMessage(text string) error {
	form := url.Values{}
	form.Set("chat_id", t.chatID)
	form.Set("text", text)
	form.Set("parse_mode", "HTML")
	form.Set("disable_web_page_preview", "true")

	req, err := http.NewRequest(http.MethodPost,
		"https://api.telegram.org/bot"+t.botToken+"/sendMessage",
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram send: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram send: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// AlertManager — проверка порогов + дедуп + отправка. Собственной горутины
// нет: вызывается коллектором после каждого цикла сбора (и после записи
// снапшота каждой панели). Часы — a.now (инъекция для детерминированных тестов).
type AlertManager struct {
	db     *MetricsDB
	sender tgSender
	cfg    AlertConfig
	log    *log.Logger
	now    func() time.Time
}

// NewAlertManager создаёт менеджер алертов.
func NewAlertManager(db *MetricsDB, sender tgSender, cfg AlertConfig, logger *log.Logger) *AlertManager {
	if logger == nil {
		logger = log.Default()
	}
	return &AlertManager{db: db, sender: sender, cfg: cfg, log: logger, now: time.Now}
}

func alertKey(typ, id string) string { return typ + ":" + id }

// CheckPanel проверяет пороги по свежему снапшоту панели (после успешной
// записи). Заодно снимает panel_down — панель снова отдаёт данные.
func (a *AlertManager) CheckPanel(p model.Panel, rec SnapshotRecord) {
	now := a.now()

	// Панель жива — возврат в норму по «недоступности».
	a.recover(alertKey(alertPanelDown, p.ID), a.msgPanelUp(p), now)

	// RAM: mem_max (max — для алертов), fallback — mem_avg.
	if v := alertValue(rec.MemMax, rec.MemAvg); v != nil {
		if *v > a.cfg.RAMThresholdPct {
			a.fire(alertKey(alertRAM, p.ID), a.msgRAM(p, *v), now)
		} else {
			a.recover(alertKey(alertRAM, p.ID), a.msgRAMOK(p, *v), now)
		}
	}

	// Load1: порог = ядра × фактор.
	if rec.Load1Avg != nil {
		limit := a.cfg.LoadCores * a.cfg.LoadFactor
		if *rec.Load1Avg > limit {
			a.fire(alertKey(alertLoad, p.ID), a.msgLoad(p, *rec.Load1Avg, limit), now)
		} else {
			a.recover(alertKey(alertLoad, p.ID), a.msgLoadOK(p, *rec.Load1Avg), now)
		}
	}

	// Трафик: текущее окно против среднего за TrafficWindow. Мало истории —
	// ни алерта, ни «ок» (условие не оценимо).
	if rec.NetUp != nil || rec.NetDown != nil {
		avgUp, avgDown, nUp, nDown := a.trafficBaseline(p.ID, rec.TS, now)
		minN := a.cfg.TrafficMinSamples
		upSpike := rec.NetUp != nil && nUp >= minN && avgUp > 0 &&
			float64(*rec.NetUp) > a.cfg.TrafficMultiplier*avgUp &&
			float64(*rec.NetUp) > a.cfg.TrafficMinMbs*1024*1024*float64(snapshotWindowSec)
		downSpike := rec.NetDown != nil && nDown >= minN && avgDown > 0 &&
			float64(*rec.NetDown) > a.cfg.TrafficMultiplier*avgDown &&
			float64(*rec.NetDown) > a.cfg.TrafficMinMbs*1024*1024*float64(snapshotWindowSec)

		key := alertKey(alertTraffic, p.ID)
		if upSpike || downSpike {
			a.fire(key, a.msgTraffic(p, rec, avgUp, avgDown, upSpike, downSpike), now)
		} else if nUp >= minN || nDown >= minN {
			a.recover(key, a.msgTrafficOK(p), now)
		}
	}
}

// CheckPanelDown фиксирует недоступность панели (вызов из коллектора на пути
// ошибки). Чтобы не алертить на свежедобавленной панели, которая ещё ни разу
// не отдала данные, требуем наличие хотя бы одного снапшота в истории.
func (a *AlertManager) CheckPanelDown(p model.Panel) {
	key := alertKey(alertPanelDown, p.ID)
	st, err := a.db.GetAlertState(key)
	if err != nil {
		a.log.Printf("alerts: state %s: %v", key, err)
		return
	}
	if st == nil {
		if _, ok := a.db.MaxSnapshotTS(p.ID); !ok {
			return // панель никогда не отдавала телеметрию — не алертим
		}
	}
	a.fire(key, a.msgPanelDown(p), a.now())
}

// CheckTesters проверяет «тестер не отвечает» по last_heartbeat_at. Тестеры,
// которые ни разу не контактировали (heartbeat NULL), пропускаются —
// это «неизвестно», а не «просрочено».
func (a *AlertManager) CheckTesters() {
	now := a.now()
	testers, err := a.db.ListTesters()
	if err != nil {
		a.log.Printf("alerts: testers: %v", err)
		return
	}
	for _, t := range testers {
		if t.Enabled == 0 {
			continue
		}
		if t.LastHeartbeatAt == nil || *t.LastHeartbeatAt == "" {
			continue
		}
		hb, err := time.Parse(runTimeFormat, *t.LastHeartbeatAt)
		if err != nil {
			a.log.Printf("alerts: heartbeat тестера %s: %v", t.Name, err)
			continue
		}
		key := alertKey(alertTesterDead, t.Name)
		if age := now.Sub(hb); age > a.cfg.StaleTesterAfter {
			a.fire(key, a.msgTesterStale(t, age), now)
		} else {
			a.recover(key, a.msgTesterOK(t), now)
		}
	}
}

// fire — условие болит: шлём алерт, но не чаще раза в cooldown. Состояние
// помечается «болит» даже если отправка не удалась (LastFiredAt не двигается —
// следующий цикл попробует снова).
func (a *AlertManager) fire(key, text string, now time.Time) {
	if !a.cfg.Enabled {
		return
	}
	st, err := a.db.GetAlertState(key)
	if err != nil {
		a.log.Printf("alerts: state %s: %v", key, err)
		return
	}
	if st == nil {
		st = &AlertState{Key: key}
	}
	cd := int64(a.cfg.Cooldown.Seconds())
	if now.Unix()-st.LastFiredAt >= cd {
		if err := a.send(text); err != nil {
			a.log.Printf("alerts: отправка %s: %v", key, err)
		} else {
			st.LastFiredAt = now.Unix()
		}
	}
	st.State = 1
	st.UpdatedAt = now.Unix()
	if err := a.db.SetAlertState(st); err != nil {
		a.log.Printf("alerts: state %s: %v", key, err)
	}
}

// recover — условие вернулось в норму: «ок»-сообщение, тоже не чаще cooldown.
// Состояние сбрасывается даже если OK не отправлен (молчаливое восстановление).
func (a *AlertManager) recover(key, text string, now time.Time) {
	if !a.cfg.Enabled {
		return
	}
	st, err := a.db.GetAlertState(key)
	if err != nil {
		a.log.Printf("alerts: state %s: %v", key, err)
		return
	}
	if st == nil || st.State == 0 {
		return // не болело — «ок» не нужен
	}
	cd := int64(a.cfg.Cooldown.Seconds())
	if now.Unix()-st.LastOKAt >= cd {
		if err := a.send(text); err != nil {
			a.log.Printf("alerts: отправка OK %s: %v", key, err)
		} else {
			st.LastOKAt = now.Unix()
		}
	}
	st.State = 0
	st.UpdatedAt = now.Unix()
	if err := a.db.SetAlertState(st); err != nil {
		a.log.Printf("alerts: state %s: %v", key, err)
	}
}

// send отправляет сообщение (nil-sender — заглушка, отправки нет).
func (a *AlertManager) send(text string) error {
	if a.sender == nil {
		return nil
	}
	return a.sender.SendMessage(text)
}

// trafficBaseline считает средние net_up/net_down за TrafficWindow по
// снапшотам СТАРШЕ текущего окна (recTS) — текущее окно в базлайн не входит.
func (a *AlertManager) trafficBaseline(panelID string, recTS int64, now time.Time) (avgUp, avgDown float64, nUp, nDown int) {
	from := now.Add(-a.cfg.TrafficWindow).Unix()
	to := recTS - 1
	rows, err := a.db.Snapshots(panelID, from, to)
	if err != nil {
		a.log.Printf("alerts: базлайн трафика %s: %v", panelID, err)
		return 0, 0, 0, 0
	}
	var sumUp, sumDown float64
	for _, r := range rows {
		if r.NetUp != nil {
			sumUp += float64(*r.NetUp)
			nUp++
		}
		if r.NetDown != nil {
			sumDown += float64(*r.NetDown)
			nDown++
		}
	}
	if nUp > 0 {
		avgUp = sumUp / float64(nUp)
	}
	if nDown > 0 {
		avgDown = sumDown / float64(nDown)
	}
	return avgUp, avgDown, nUp, nDown
}

// alertValue — значение для алерта: max (для алертов), fallback — avg.
func alertValue(max, avg *float64) *float64 {
	if max != nil {
		return max
	}
	return avg
}

// --- Сообщения (HTML, экранируем только переменные части) ---

func esc(s string) string { return html.EscapeString(s) }

// fmtBytes — человекочитаемый размер (1024-байтные единицы, как humanize).
func fmtBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func (a *AlertManager) msgRAM(p model.Panel, mem float64) string {
	return fmt.Sprintf("🚨 <b>Панель %s</b>: RAM %.0f%% (порог %g%%)",
		esc(p.Name), mem, a.cfg.RAMThresholdPct)
}

func (a *AlertManager) msgRAMOK(p model.Panel, mem float64) string {
	return fmt.Sprintf("✅ <b>Панель %s</b>: RAM в норме (%.0f%%)", esc(p.Name), mem)
}

func (a *AlertManager) msgLoad(p model.Panel, load, limit float64) string {
	return fmt.Sprintf("🚨 <b>Панель %s</b>: load1 %.2f (порог %.0f ядер × %.1f = %.2f)",
		esc(p.Name), load, a.cfg.LoadCores, a.cfg.LoadFactor, limit)
}

func (a *AlertManager) msgLoadOK(p model.Panel, load float64) string {
	return fmt.Sprintf("✅ <b>Панель %s</b>: load1 в норме (%.2f)", esc(p.Name), load)
}

func (a *AlertManager) msgTraffic(p model.Panel, rec SnapshotRecord, avgUp, avgDown float64, upSpike, downSpike bool) string {
	var parts []string
	if upSpike && rec.NetUp != nil {
		parts = append(parts, fmt.Sprintf("↑ %s (среднее %s, ×%.1f)",
			fmtBytes(*rec.NetUp), fmtBytes(int64(avgUp)), float64(*rec.NetUp)/avgUp))
	}
	if downSpike && rec.NetDown != nil {
		parts = append(parts, fmt.Sprintf("↓ %s (среднее %s, ×%.1f)",
			fmtBytes(*rec.NetDown), fmtBytes(int64(avgDown)), float64(*rec.NetDown)/avgDown))
	}
	return fmt.Sprintf("🚨 <b>Панель %s</b>: аномальный трафик за окно: %s",
		esc(p.Name), strings.Join(parts, ", "))
}

func (a *AlertManager) msgTrafficOK(p model.Panel) string {
	return fmt.Sprintf("✅ <b>Панель %s</b>: трафик в норме", esc(p.Name))
}

func (a *AlertManager) msgPanelDown(p model.Panel) string {
	return fmt.Sprintf("🚨 <b>Панель %s</b>: не отдаёт телеметрию (недоступна)", esc(p.Name))
}

func (a *AlertManager) msgPanelUp(p model.Panel) string {
	return fmt.Sprintf("✅ <b>Панель %s</b>: снова отдаёт телеметрию", esc(p.Name))
}

func (a *AlertManager) msgTesterStale(t Tester, age time.Duration) string {
	return fmt.Sprintf("🚨 <b>Тестер %s</b>: не отвечает %s (последний контакт %s)",
		esc(t.Name), age.Round(time.Minute), esc(*t.LastHeartbeatAt))
}

func (a *AlertManager) msgTesterOK(t Tester) string {
	return fmt.Sprintf("✅ <b>Тестер %s</b>: снова отвечает", esc(t.Name))
}
