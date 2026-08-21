// Форматтеры для отображения байтов и дат.

export function fmtBytes(b) {
  if (b == null || isNaN(b)) return '—';
  const u = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];
  let i = 0, v = Number(b);
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return (v >= 100 ? v.toFixed(0) : v.toFixed(1)) + u[i];
}

export function fmtDate(isoOrMs) {
  if (!isoOrMs) return '—';
  let d;
  if (typeof isoOrMs === 'number' || /^\d+$/.test(String(isoOrMs))) {
    d = new Date(Number(isoOrMs));
  } else if (isoOrMs.length === 10) {
    d = new Date(isoOrMs + 'T00:00:00');
  } else {
    d = new Date(isoOrMs);
  }
  if (isNaN(d.getTime())) return '—';
  return d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric' });
}

export function fmtShortDate(isoOrMs) {
  if (!isoOrMs) return '—';
  let d;
  if (typeof isoOrMs === 'number' || /^\d+$/.test(String(isoOrMs))) {
    d = new Date(Number(isoOrMs));
  } else if (isoOrMs.length === 10) {
    d = new Date(isoOrMs + 'T00:00:00');
  } else {
    d = new Date(isoOrMs);
  }
  if (isNaN(d.getTime())) return '—';
  return d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit' });
}

export function fmtDateTime(s) {
  if (!s) return '—';
  const d = new Date(s);
  if (isNaN(d.getTime())) return s;
  return d.toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' });
}
