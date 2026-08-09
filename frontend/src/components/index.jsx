import React, { createContext, useContext, useState, useCallback, useRef, useEffect } from 'react';

// ─── Toast System ───
const ToastContext = createContext(null);

export function ToastProvider({ children }) {
  const [toasts, setToasts] = useState([]);
  const idCounter = useRef(0);

  const showToast = useCallback((message, type, duration = 3000) => {
    const id = ++idCounter.current;
    setToasts(prev => [...prev, { id, message, type }]);
    const ms = typeof duration === 'number' && duration > 0 ? duration : 3000;
    setTimeout(() => {
      setToasts(prev => prev.filter(t => t.id !== id));
    }, ms);
  }, []);

  return (
    <ToastContext.Provider value={showToast}>
      {children}
      <div className="toast-container">
        {toasts.map(t => (
          <div key={t.id} className={`toast ${t.type || ''}`}>{t.message}</div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  return useContext(ToastContext);
}

// ─── Modal ───
export function Modal({ title, children, onClose, wide }) {
  useEffect(() => {
    const handleEsc = (e) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', handleEsc);
    return () => window.removeEventListener('keydown', handleEsc);
  }, [onClose]);

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className={`modal-content ${wide ? 'modal-wide' : ''}`} onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <h3>{title}</h3>
          <button className="btn btn-icon modal-close" onClick={onClose}>✕</button>
        </div>
        <div className="modal-body">
          {children}
        </div>
      </div>
    </div>
  );
}

// ─── Formatting helpers ───
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

// ─── Header ───
export function Header({ panels, selectedPanelId, onPanelChange, onAddPanel, onDeletePanel, onSyncAll, syncing }) {
  return (
    <header className="header">
      <div className="header-left">
        <span className="app-title">Vless<span className="accent">Panel</span></span>
        <button className="btn btn-danger btn-sm" disabled={panels.length === 0} onClick={onDeletePanel}>– Панель</button>
        <button className="btn btn-primary btn-sm" onClick={onAddPanel}>+ Панель</button>
      </div>
      <div className="header-right">
        <button className="btn btn-sm btn-success" onClick={onSyncAll} disabled={syncing}>
          {syncing ? '⟳ Синхронизация…' : '⟳ Синк с агрегатором'}
        </button>
        <span className="app-version">v0.9.0</span>
      </div>
    </header>
  );
}

// ─── Client card with inbound chips (left column) ───
export function ClientCard({ client, inbounds, keySources, activeSubKeys, onChipClick, onOpenClient, panelName, panelId }) {
  const chipInbounds = (inbounds || []).filter(ib => (client.inboundIds || []).includes(ib.id));

  const statusFor = (inboundId) => {
    const ks = (keySources || []).find(k =>
      k.type === 'panel' && k.panelId === panelId && k.clientEmail === client.email && k.inboundId === inboundId);
    return ks || null;
  };

  const expiryMs = client.expiryTime || 0;
  const expiryStr = expiryMs > 0 ? ' · до ' + fmtShortDate(expiryMs) : '';
  const trafficStr = (client.up || client.down) ? ` · ↑${fmtBytes(client.up)} ↓${fmtBytes(client.down)}` : '';

  return (
    <div className="client-card" onClick={() => onOpenClient && onOpenClient(client)}>
      <div className="client-top">
        <div>
          <div className="client-name">
            {client.email}
            {client.enable ? <span className="badge ok">вкл</span> : <span className="badge">выкл</span>}
          </div>
          <div className="client-inbounds">
            {chipInbounds.length} {chipInbounds.length === 1 ? 'инбаунд' : (chipInbounds.length < 5 ? 'инбаунда' : 'инбаундов')}
            {expiryStr}{trafficStr}
          </div>
        </div>
      </div>
      <div className="client-chips">
        {chipInbounds.length === 0 ? (
          <div className="client-noinb">нет инбаундов — привяжите на панели</div>
        ) : chipInbounds.map(ib => {
          const ks = statusFor(ib.id);
          const st = ks ? ks.status : 'ok'; // без KeySource — статус неизвестен, показываем как ok
          const added = !!(ks && activeSubKeys && activeSubKeys.has(ks.id));
          const inactive = ib.enable === false;
          return (
            <button
              key={ib.id}
              className={`inb-chip${added ? ' added' : ''}${inactive ? ' inactive' : ''}`}
              title={`${panelName} · ${ib.remark} :${ib.port} · ${client.email}${inactive ? ' · ⚠️ инбаунд неактивен' : ''}${ks && ks.expireDate ? ' · до ' + fmtDate(ks.expireDate) : ''}${added ? '\nуже добавлено в подписку' : ''}`}
              onClick={(e) => { e.stopPropagation(); onChipClick && onChipClick(client, ib); }}
            >
              <span className={`idot ${st}${inactive ? ' inactive' : ''}`}></span>
              <span className="inb-name">{ib.remark}</span>
              <span className="inb-port">:{ib.port}</span>
              {added && <span className="inb-ok">✓</span>}
            </button>
          );
        })}
      </div>
    </div>
  );
}

// ─── KeySource chip (right column) ───
export function KSChip({ subKey, keySource, onOpen, onCopy, onDelete, onTest, testing }) {
  const ks = keySource || null;

  // manual / legacy — grey chip
  if (!ks) {
    return (
      <div className="ks-chip ks-manual" title="manual · клик — детали" onClick={onOpen}>
        <span className="ks-dot manual"></span>
        <span className="ks-label">
          <span className="ks-server ell">manual</span>
          <span className="ks-sep">·</span>
          <span className="ks-inbound ell">{shortLink(subKey.link)}</span>
        </span>
        <span className="ks-meta">
          <span className="ks-status manual">manual</span>
          <button className="ks-ico js-copy" title="Скопировать ключ" onClick={(e) => { e.stopPropagation(); onCopy(subKey.link); }}>⧉</button>
          <button className="ks-ico del js-del" title="Удалить из подписки" onClick={(e) => { e.stopPropagation(); onDelete(subKey); }}>×</button>
        </span>
      </div>
    );
  }

  const st = ks.status || 'ok';
  const cls = 'ks-chip ' + (st === 'ok' ? '' : st === 'expired' ? 'ks-expired' : st === 'manual' ? 'ks-manual' : 'ks-missing');
  const statusTxt =
    st === 'ok' ? <span className="ks-status ok">ok</span>
    : st === 'expired' ? <span className="ks-status expired">закончился</span>
    : st === 'manual' ? <span className="ks-status manual">manual</span>
    : <span className="ks-status missing">{st === 'missing' ? 'missing' : 'панель недоступна'}</span>;

  let center = null;
  if (st === 'ok') {
    center = (
      <>
        <span className="ks-traffic" title="Трафик (clientStats 3X-UI)">
          ↑{fmtBytes(ks.traffic && ks.traffic.up)} ↓{fmtBytes(ks.traffic && ks.traffic.down)}
        </span>
        {ks.expireDate && <span className="ks-expiry" title="Окончание">до {fmtShortDate(ks.expireDate)}</span>}
      </>
    );
  } else if (st === 'expired') {
    center = (
      <>
        <svg className="ico-clock" width="14" height="14" viewBox="0 0 16 16" title={`Срок истёк ${fmtDate(ks.expireDate)}`}>
          <circle cx="8" cy="8" r="7" fill="none" stroke="currentColor" strokeWidth="1.6"/>
          <path d="M8 4.5V8l2.5 1.5" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"/>
        </svg>
        {ks.expireDate && <span className="ks-expiry">истёк {fmtShortDate(ks.expireDate)}</span>}
      </>
    );
  } else {
    center = (
      <>
        <svg className="ico-warn" width="14" height="14" viewBox="0 0 16 16">
          <path d="M8 1.5 15 14H1z" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round"/>
          <path d="M8 6v3.5" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"/>
          <circle cx="8" cy="12" r="0.9" fill="currentColor"/>
        </svg>
        <span className="ks-note">{ks.error || (st === 'missing' ? 'клиент/инбаунд не найден' : 'панель недоступна')}</span>
      </>
    );
  }

  const testRes = ks.lastTest;
  let testHtml;
  if (testing) {
    testHtml = <span className="ks-testres run"><span className="spin small"></span> тест…</span>;
  } else {
    let resTxt = '', resCls = '';
    if (testRes) {
      resCls = testRes.status === 'ok' ? 'ok' : 'fail';
      resTxt = testRes.status === 'ok'
        ? '✓ ' + (testRes.ms != null ? testRes.ms + ' мс' : 'ok')
        : '✗ ' + (testRes.error || 'ошибка');
    }
    testHtml = (
      <>
        <button className="ks-ico js-test" title="Тест ключа (test-single)" onClick={(e) => { e.stopPropagation(); onTest(ks); }}>🧪</button>
        {resTxt && <span className={`ks-testres ${resCls}`} title="Тест демоном vlesssubtest">{resTxt}</span>}
      </>
    );
  }

  return (
    <div className={cls} data-ks={ks.id} title="Клик — детали KeySource" onClick={onOpen}>
      <span className={`ks-dot ${st === 'manual' ? 'manual' : st === 'expired' ? 'expired' : st === 'ok' ? 'ok' : 'missing'}`}></span>
      <span className="ks-label">
        <span className="ks-server">{ks.panelName || '?'}</span>
        <span className="ks-sep">·</span>
        <span className="ks-inbound">{ks.inboundRemark || '—'}{ks.inboundPort ? ' :' + ks.inboundPort : ''}</span>
        <span className="ks-sep">·</span>
        <span className="ks-email">{ks.clientEmail}</span>
      </span>
      <span className="ks-meta">
        {statusTxt}
        {center}
        {testHtml}
        <button className="ks-ico js-copy" title="Скопировать vless-ключ" onClick={(e) => { e.stopPropagation(); onCopy(ks); }}>⧉</button>
        <button className="ks-ico del js-del" title="Удалить из подписки" onClick={(e) => { e.stopPropagation(); onDelete(subKey); }}>×</button>
      </span>
    </div>
  );
}

function shortLink(link) {
  if (!link) return '—';
  return link.length > 60 ? link.slice(0, 60) + '…' : link;
}

// ─── Modals ───

// New subscription (name + duplicate check)
export function NewSubModal({ onClose, onSubmit, existingNames, hint }) {
  const [name, setName] = useState('');
  const [error, setError] = useState('');

  const submit = () => {
    const n = name.trim();
    if (!n) { setError('Укажите имя подписки'); return; }
    if (existingNames.some(x => x.toLowerCase() === n.toLowerCase())) {
      setError(`Подписка с именем «${n}» уже существует`);
      return;
    }
    onSubmit(n);
  };

  return (
    <Modal title="➕ Новая подписка" onClose={onClose}>
      <div className="modal-form">
        <div className="form-group">
          <label htmlFor="newSubName">Имя подписки</label>
          <input id="newSubName" value={name} onChange={e => setName(e.target.value)}
            placeholder="Например: FriendsFamily, perMonth…" maxLength="60" autoFocus
            onKeyDown={e => { if (e.key === 'Enter') submit(); }} />
          {error && <div className="form-error">⚠ {error}</div>}
          <div className="form-hint">Имя станет частью файла и ссылки: <code className="mono">configs-{name || '{имя}'}.txt</code> · <code className="mono">/sub/{name || '{имя}'}</code></div>
          {hint && <div className="form-hint">{hint}</div>}
        </div>
        <div className="modal-actions">
          <button className="btn" onClick={onClose}>Отмена</button>
          <button className="btn btn-primary" onClick={submit}>Создать черновик</button>
        </div>
      </div>
    </Modal>
  );
}

// KeySource details
export function KSDetailsModal({ keySource, usedInSubs, inThisSub, onClose, onCopyKey, onDelete, onTest, testing, subKey }) {
  const ks = keySource;
  const manual = !ks || ks.type === 'manual';

  const statusBadge = !ks ? <span className="badge">manual</span> : (
    ks.status === 'ok' ? <span className="badge ok">● ok</span>
    : ks.status === 'expired' ? <span className="badge danger">🕐 закончился</span>
    : ks.status === 'panel_unreachable' ? <span className="badge warn">▲ панель недоступна</span>
    : ks.status === 'missing' ? <span className="badge warn">▲ missing — не извлечётся</span>
    : <span className="badge">manual</span>
  );

  const lastTest = ks && ks.lastTest
    ? (ks.lastTest.status === 'ok'
        ? `✅ OK за ${ks.lastTest.ms || '?'} мс · ${fmtDateTime(ks.lastTest.at)}`
        : `❌ ${ks.lastTest.error || 'ошибка'} · ${fmtDateTime(ks.lastTest.at)}`)
    : '— тест не запускался';

  const link = (ks && ks.cachedKey && ks.cachedKey.link) || (ks && ks.vlessLink) || (subKey && subKey.link);

  const rows = [];
  if (!ks) {
    rows.push(['Тип', 'manual (хранит строку vless-ключа)']);
    rows.push(['Статус', statusBadge]);
    rows.push(['Последний тест', lastTest]);
    rows.push(['Генерация', 'включается всегда, не трогается при перегенерации']);
  } else if (ks.type === 'manual') {
    rows.push(['Тип', 'KeySource · manual']);
    rows.push(['Label', ks.label || 'manual']);
    rows.push(['Статус', statusBadge]);
    rows.push(['Последний тест', lastTest]);
    rows.push(['Генерация', 'включается всегда']);
    rows.push(['Используется', usedInSubs > 0 ? `в ${usedInSubs} ${usedInSubs === 1 ? 'подписке' : 'подписках'}` : 'нигде']);
  } else {
    rows.push(['Тип', 'KeySource · panel (3X-UI)']);
    rows.push(['Сервер', `${ks.panelName || '—'}`]);
    rows.push(['Инбаунд', `${ks.inboundRemark || '—'}${ks.inboundPort ? ' :' + ks.inboundPort : ''}`]);
    rows.push(['Клиент', ks.clientEmail || '—']);
    rows.push(['Статус', statusBadge]);
    if (ks.expireDate) rows.push(['Окончание', fmtDate(ks.expireDate)]);
    if (ks.traffic) rows.push(['Трафик (up/down)', `↑ ${fmtBytes(ks.traffic.up)} · ↓ ${fmtBytes(ks.traffic.down)}`]);
    rows.push(['Последний тест', lastTest]);
    rows.push(['Кеш ключа', ks.cachedKey ? `получен ${fmtDateTime(ks.cachedKey.fetchedAt)} (TTL 10 мин)` : 'нет кеша — ключ извлечётся при генерации']);
    rows.push(['Используется', usedInSubs > 0 ? `в ${usedInSubs} ${usedInSubs === 1 ? 'подписке' : 'подписках'}` : 'нигде']);
  }

  return (
    <Modal title={`🔑 ${ks ? (ks.label || `${ks.panelName || ''} · ${ks.clientEmail || 'manual'}`) : 'manual'}`} onClose={onClose}>
      <div className="modal-form">
        <div className="ks-info-grid">
          {rows.map(([k, v]) => (
            <React.Fragment key={k}>
              <span className="k">{k}</span>
              <span className="v">{v}</span>
            </React.Fragment>
          ))}
        </div>
        {link && (
          <div className="ks-keybox mono" title={link}>{link.length > 92 ? link.slice(0, 92) + '…' : link}</div>
        )}
        <div className="modal-actions">
          <button className="btn" onClick={onClose}>Закрыть</button>
          {!manual && (
            <button className="btn" style={{ color: 'var(--danger)' }} onClick={onDelete}>Удалить KeySource</button>
          )}
          {!manual && (
            <button className="btn" onClick={onTest} disabled={testing}>
              {testing ? '⏳ Тест…' : '🧪 Тест ключа'}
            </button>
          )}
          <button className="btn btn-success" onClick={() => onCopyKey(ks)}>⧉ Скопировать vless-ключ</button>
        </div>
      </div>
    </Modal>
  );
}

// Delete subscription confirmation
export function DeleteSubModal({ sub, onClose, onConfirm }) {
  const n = (sub && sub.keys ? sub.keys.length : 0);
  return (
    <Modal title="🗑 Удалить подписку" onClose={onClose}>
      <div className="danger-box">
        Точно удалить подписку <b>«{sub.name}»</b>?
        <br />
        Будут удалены {n} {n === 1 ? 'ключ' : (n < 5 ? 'ключа' : 'ключей')}
        {sub.status === 'active' ? <> и локальный файл <code className="mono">configs-{sub.name}.txt</code></> : ' (файл не создан)'}.
      </div>
      <div className="modal-actions">
        <button className="btn" onClick={onClose}>Отмена</button>
        <button className="btn btn-danger" onClick={onConfirm}>Удалить</button>
      </div>
    </Modal>
  );
}

// Delete KeySource confirmation (used in N subscriptions warning)
export function DeleteKSModal({ keySource, usedInSubs, subNames, onClose, onConfirm }) {
  const label = keySource ? (keySource.label || `${keySource.panelName || ''} · ${keySource.clientEmail || keySource.id}`) : '';
  return (
    <Modal title="🗑 Удалить KeySource" onClose={onClose}>
      <div className="modal-form">
        {usedInSubs > 0 ? (
          <div className="warn-box">
            ⚠ KeySource <b>{label}</b> используется в <b>{usedInSubs}</b> {usedInSubs === 1 ? 'подписке' : 'подписках'}:
            {(subNames || []).map(s => ` «${s}»`).join(',')}.
            <br />
            Он будет удалён из всех. Конвертации в manual нет — при необходимости скопируйте ключ заранее.
          </div>
        ) : (
          <div className="form-hint">KeySource не используется ни в одной подписке.</div>
        )}
        <div className="modal-actions">
          <button className="btn" onClick={onClose}>Отмена</button>
          <button className="btn btn-danger" onClick={onConfirm}>Точно удалить</button>
        </div>
      </div>
    </Modal>
  );
}

// Generation report (partial success)
export function ReportModal({ subName, report, included, skipped, onClose }) {
  return (
    <Modal title={`📄 ${subName} — результат`} onClose={onClose}>
      <div className="modal-form">
        <div className="report-list">
          {(report || []).map((r, i) => (
            <div key={i} className={`report-item ${r.kind}`}>
              <span className="mark">
                {r.kind === 'ok' ? '✅' : r.kind === 'manual' ? '🔘' : '⏭'}
              </span>
              <span>
                {r.label}
                {r.kind === 'ok' && <span className="why"> — ключ извлечён{r.ms ? ` (${r.ms} мс)` : ''}</span>}
                {r.kind === 'manual' && <span className="why"> — manual, сохранён как есть</span>}
                {r.kind === 'skip' && <span className="why"> — {r.why}</span>}
              </span>
            </div>
          ))}
        </div>
        {(included != null || skipped != null) && (
          <div className="form-hint">
            Включено: <b>{included || 0}</b> · пропущено: <b>{skipped || 0}</b>
          </div>
        )}
        <div className="form-hint">
          Файл: <code className="mono">configs-{subName}.txt</code> · ссылка: <code className="mono">https://example.com/sub/{subName}</code>
          <br />Не забудьте синхронизировать с агрегатором.
        </div>
        <div className="modal-actions">
          <button className="btn btn-primary" onClick={onClose}>Понятно</button>
        </div>
      </div>
    </Modal>
  );
}

// ─── Panel / client management modals (kept from previous UI) ───
export function AddPanelModal({ onClose, onSubmit }) {
  const [name, setName] = React.useState('');
  const [url, setUrl] = React.useState('');
  const [token, setToken] = React.useState('');
  const [webBasePath, setWebBasePath] = React.useState('');

  const handleSubmit = (e) => {
    e.preventDefault();
    if (!name || !url || !token) return;
    onSubmit({ name, url: url.replace(/\/+$/, ''), token, webBasePath: webBasePath.replace(/\/+$/, '') });
  };

  return (
    <Modal title="➕ Добавить панель" onClose={onClose}>
      <form onSubmit={handleSubmit} className="modal-form">
        <div className="form-group">
          <label>Название</label>
          <input type="text" value={name} onChange={e => setName(e.target.value)} placeholder="Hip Warsaw" required autoFocus />
        </div>
        <div className="form-group">
          <label>URL</label>
          <input type="text" value={url} onChange={e => setUrl(e.target.value)} placeholder="https://203.0.113.4:2053" required />
        </div>
        <div className="form-group">
          <label>Web Base Path</label>
          <input type="text" value={webBasePath} onChange={e => setWebBasePath(e.target.value)} placeholder="/abcdefgh12345678" />
        </div>
        <div className="form-group">
          <label>Token</label>
          <input type="text" value={token} onChange={e => setToken(e.target.value)} placeholder="Bearer token" required />
        </div>
        <div className="modal-actions">
          <button type="submit" className="btn btn-primary">➕ Добавить</button>
          <button type="button" className="btn" onClick={onClose}>Отмена</button>
        </div>
      </form>
    </Modal>
  );
}

export function AddClientModal({ onClose, onSubmit, inbounds }) {
  const [email, setEmail] = React.useState('');
  const [inboundId, setInboundId] = React.useState(inbounds[0]?.id || '');
  const [expiryDate, setExpiryDate] = React.useState('');

  const handleSubmit = (e) => {
    e.preventDefault();
    if (!email || !inboundId) return;
    onSubmit({ email, inboundId: parseInt(inboundId), expiryDate });
  };

  return (
    <Modal title="👤 Новый клиент" onClose={onClose}>
      <form onSubmit={handleSubmit} className="modal-form">
        <div className="form-group">
          <label>Email клиента</label>
          <input type="text" value={email} onChange={e => setEmail(e.target.value)} placeholder="client@email.com" required autoFocus />
        </div>
        <div className="form-group">
          <label>Инбаунд</label>
          <select value={inboundId} onChange={e => setInboundId(e.target.value)}>
            {(inbounds || []).map(ib => (
              <option key={ib.id} value={ib.id}>{ib.remark} (:{ib.port})</option>
            ))}
          </select>
        </div>
        <div className="form-group">
          <label>Дата окончания (опционально, полночь UTC)</label>
          <input type="date" value={expiryDate} onChange={e => setExpiryDate(e.target.value)} />
        </div>
        <div className="modal-actions">
          <button type="submit" className="btn btn-primary">➕ Создать</button>
          <button type="button" className="btn" onClick={onClose}>Отмена</button>
        </div>
      </form>
    </Modal>
  );
}

export function EditClientModal({ client, allInbounds, onClose, onAttachInbound, onDetachInbound, onSave }) {
  const inbounds = client.inbounds || [];
  const attachedIds = new Set(inbounds);
  const available = (allInbounds || []).filter(ib => !attachedIds.has(ib.remark));
  const [addInboundId, setAddInboundId] = React.useState(available[0] ? String(available[0].id) : '');
  // Если список доступных инбаундов изменился (например, после добавления),
  // переводим селект на актуальный первый доступный, а не на устаревший id.
  React.useEffect(() => {
    if (!available.some(ib => String(ib.id) === addInboundId)) {
      setAddInboundId(available[0] ? String(available[0].id) : '');
    }
  }, [available, addInboundId]);
  const expiryMillis = client.expiryTime || 0;
  const expiryStr = expiryMillis > 0
    ? new Date(expiryMillis).toISOString().slice(0, 10)
    : '';
  const [expiryDate, setExpiryDate] = React.useState(expiryStr);

  return (
    <Modal title="✏️ Редактировать клиента" onClose={onClose}>
      <div className="modal-form">
        <div className="form-group">
          <label>Email</label>
          <input type="text" value={client.email} disabled style={{ opacity: 0.6 }} />
        </div>

        <div className="form-group">
          <label>Инбаунды</label>
          <div className="edit-inbound-list">
            {inbounds.map(inb => {
              const inbObj = (allInbounds || []).find(ib => ib.remark === inb);
              const inbInactive = !!(inbObj && inbObj.enable === false);
              return (
                <span key={inb} className={`key-chip edit-inbound-chip${inbInactive ? ' inactive' : ''}`} title={inbInactive ? `${inb} — инбаунд неактивен` : undefined}>
                  {inb}
                  <button
                    className="edit-inbound-remove"
                    onClick={() => onDetachInbound(client.email, (allInbounds || []).find(ib => ib.remark === inb)?.id)}
                    title="Удалить инбаунд"
                  >&times;</button>
                </span>
              );
            })}
          </div>
          {available.length > 0 && (
            <div className="edit-inbound-add">
              <select value={addInboundId} onChange={e => setAddInboundId(e.target.value)} style={{ flex: 1 }}>
                {available.map(ib => (
                  <option key={ib.id} value={ib.id}>{ib.remark} (:{ib.port})</option>
                ))}
              </select>
              <button
                className="btn btn-primary btn-sm"
                onClick={() => { onAttachInbound(client.email, parseInt(addInboundId)); }}
              >Добавить</button>
            </div>
          )}
          {inbounds.length === 0 && <p className="form-hint">Нет привязанных инбаундов</p>}
        </div>

        <div className="form-group">
          <label>Дата окончания (полночь UTC)</label>
          <input type="date" value={expiryDate} onChange={e => setExpiryDate(e.target.value)} />
        </div>

        <div className="modal-actions">
          <button className="btn btn-primary" onClick={() => onSave(client.email, expiryDate)}>💾 Сохранить</button>
          <button className="btn" onClick={onClose}>Отмена</button>
        </div>
      </div>
    </Modal>
  );
}
