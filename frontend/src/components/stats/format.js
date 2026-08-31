// Форматирование для раздела статистики (Этап 3).
// Время в интерфейсе — Томское (UTC+7), как в моках.

const TOMSK_OFFSET = 7 * 3600 * 1000;

function pad(n) { return String(n).padStart(2, '0'); }

// unix ts (sec) → "31.08 23:05" по Томскому времени
export function fmtTomsk(ts) {
  if (!ts) return '—';
  const d = new Date(ts * 1000 + TOMSK_OFFSET);
  return `${pad(d.getUTCDate())}.${pad(d.getUTCMonth() + 1)} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}`;
}

// «31.08 23:05» (без года) или «01.01.2027 00:00» (с годом, если год ≠ текущий)
export function fmtTomskSmart(ts) {
  if (!ts) return '—';
  const d = new Date(ts * 1000 + TOMSK_OFFSET);
  const now = new Date(Date.now() + TOMSK_OFFSET);
  const base = `${pad(d.getUTCDate())}.${pad(d.getUTCMonth() + 1)} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}`;
  if (d.getUTCFullYear() !== now.getUTCFullYear()) {
    return `${pad(d.getUTCDate())}.${pad(d.getUTCMonth() + 1)}.${d.getUTCFullYear()} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}`;
  }
  return base;
}

// Длительность прогона: 118 сек → «1м 58с»
export function fmtDur(sec) {
  if (sec == null || isNaN(sec)) return '—';
  const m = Math.floor(sec / 60);
  const s = Math.round(sec % 60);
  if (m === 0) return `${s}с`;
  return `${m}м ${pad(s)}с`;
}

// байты → «5.56 GB»
export function fmtGB(bytes) {
  if (bytes == null || isNaN(bytes)) return '—';
  return (bytes / (1024 * 1024 * 1024)).toFixed(2);
}

// байты/бакет → KB/s (для графика сети)
export function kbPerSec(bytes, bucketSec) {
  if (bytes == null) return null;
  return +(bytes / bucketSec / 1024).toFixed(2);
}

// байты/бакет → MB/s (для графика сети)
export function mbPerSec(bytes, bucketSec) {
  if (bytes == null) return null;
  return +(bytes / bucketSec / 1024 / 1024).toFixed(3);
}

// Средние значения по точкам (для подписей «avg 23.6% · max 28.9%»)
export function avgOf(arr) {
  const vals = arr.filter(v => v != null);
  if (vals.length === 0) return null;
  return +(vals.reduce((a, b) => a + b, 0) / vals.length).toFixed(1);
}

export function maxOf(arr) {
  const vals = arr.filter(v => v != null);
  if (vals.length === 0) return null;
  return +Math.max(...vals).toFixed(1);
}

// Динамический максимум оси Y: чуть больше пика данных, но не меньше floor
export function yMax(arr, floor, factor = 1.2) {
  const m = maxOf(arr);
  if (m == null) return floor;
  return Math.max(floor, +(m * factor).toFixed(1));
}
