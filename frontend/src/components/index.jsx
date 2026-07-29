import React, { createContext, useContext, useState, useCallback, useRef, useEffect } from 'react';

// ─── Toast System ───
const ToastContext = createContext(null);

export function ToastProvider({ children }) {
  const [toasts, setToasts] = useState([]);
  const idCounter = useRef(0);

  const showToast = useCallback((message, duration = 2500) => {
    const id = ++idCounter.current;
    setToasts(prev => [...prev, { id, message }]);
    setTimeout(() => {
      setToasts(prev => prev.filter(t => t.id !== id));
    }, duration);
  }, []);

  return (
    <ToastContext.Provider value={showToast}>
      {children}
      <div className="toast-container">
        {toasts.map(t => (
          <div key={t.id} className="toast">{t.message}</div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  return useContext(ToastContext);
}

// ─── Modal ───
export function Modal({ title, children, onClose }) {
  useEffect(() => {
    const handleEsc = (e) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', handleEsc);
    return () => window.removeEventListener('keydown', handleEsc);
  }, [onClose]);

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content" onClick={e => e.stopPropagation()}>
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

// ─── Header ───
export function Header({ panels, selectedPanelId, onPanelChange, onAddPanel, onDeletePanel }) {
  return (
    <header className="header">
      <div className="header-left">
        {panels.length === 0 ? (
          <span className="no-panels-text">Нет панелей — добавьте первую</span>
        ) : (
          <div className="panel-select">
            <select value={selectedPanelId || ''} onChange={e => onPanelChange(e.target.value)}>
              {panels.map(p => (
                <option key={p.id} value={p.id}>{p.name} :[{new URL(p.url).hostname}]</option>
              ))}
            </select>
          </div>
        )}
        <button className="btn btn-danger btn-sm" disabled={panels.length === 0} onClick={onDeletePanel}>– Панель</button>
        <button className="btn btn-primary btn-sm" onClick={onAddPanel}>+ Панель</button>
      </div>
      <div className="header-right">
        <span className="app-title"><span className="accent">Vless</span>Panel</span>
        <span className="app-version">v0.1</span>
      </div>
    </header>
  );
}

// ─── ClientCard ───
export function ClientCard({ client, onCopyInboundKey, onClick }) {
  return (
    <div className="client-card" onClick={() => onClick && onClick(client)}>
      <div className="client-card-top">
        <div className="client-name">{client.email}</div>
      </div>
      <div className="keys-list" style={{ marginTop: 6 }}>
        {(client.inbounds || []).map((inb, idx) => (
          <span
            key={idx}
            className="key-chip"
            onClick={(e) => { e.stopPropagation(); onCopyInboundKey && onCopyInboundKey(client.email, inb); }}
          >{inb}</span>
        ))}
      </div>
    </div>
  );
}

// ─── SubscriptionCard ───
export function SubscriptionCard({
  subscription,
  isOpen,
  onToggle,
  onCopyLink,
  onRefresh,
  onDelete,
  onCopyKey,
  onDeleteKey,
  onAddKey,
  newKeyValue,
  onNewKeyChange,
  onNewKeyConfirm,
  onNewKeyCancel,
  onTest,
  showAddForm,
  testing,
  testResults,
}) {
  const parsedResults = parseTestResults(testResults || subscription.testResults);

  return (
    <div className={`sub-item ${isOpen ? 'open' : ''}`}>
      <div className="sub-header" onClick={onToggle}>
        <div>
          <span className="sub-arrow">▶</span>
          <span className="sub-title">{subscription.name}</span>
          {parsedResults && (
            <span className="badge" style={{ marginLeft: 6 }}>
              {parsedResults.okCount}/{parsedResults.totalCount} ✅
            </span>
          )}
        </div>
        <div className="sub-actions" onClick={e => e.stopPropagation()}>
          <button className="btn btn-xs btn-icon copy-sub-btn" onClick={onCopyLink} title="Копировать ссылку">📋</button>
          <button className="btn btn-xs btn-icon refresh-sub-btn" onClick={onRefresh} title="Обновить">🔄</button>
          <button className="btn btn-xs btn-icon btn-danger delete-sub-btn" onClick={onDelete} title="Удалить">🗑</button>
        </div>
      </div>
      <div className="sub-body">
        {(subscription.keys || []).map(k => (
          <div key={k.id} className="sub-key-row">
            <span className="mono" title={k.link}>{k.link}</span>
            <span
              className="copy-small copy-vless-btn"
              onClick={() => onCopyKey(k)}
            >📋</span>
            <span
              className="sub-key-del"
              onClick={() => onDeleteKey(k.id)}
              title="Удалить ключ"
            >🗑</span>
          </div>
        ))}
        <div className="sub-add-key-wrap">
          {!showAddForm ? (
            <button
              className="btn btn-sm btn-primary sub-add-key-btn"
              onClick={onAddKey}
            >➕ Добавить ключ</button>
          ) : (
            <div className="sub-add-key-form">
              <div className="sub-add-key-form-inner">
                <input
                  type="text"
                  className="sub-add-key-input"
                  placeholder="Вставьте vless:// или vmess:// ссылку..."
                  value={newKeyValue}
                  onChange={e => onNewKeyChange(e.target.value)}
                  onKeyDown={e => { if (e.key === 'Enter') onNewKeyConfirm(); }}
                  autoFocus
                />
                <button className="btn btn-sm btn-success" onClick={onNewKeyConfirm}>➕ Добавить</button>
                <button className="btn btn-sm" onClick={onNewKeyCancel}>Отмена</button>
              </div>
            </div>
          )}
        </div>
        <div className="test-section">
          <button
            className="btn btn-sm btn-success test-btn"
            onClick={onTest}
            disabled={testing}
          >{testing ? '⏳ Тестируем...' : '▶ Тест VlessSubTest'}</button>
          {parsedResults && parsedResults.rows.length > 0 && (
            <div className="test-results">
              <table className="test-table">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>IP</th>
                    <th>Remarks</th>
                    <th>Status</th>
                    <th>YT</th>
                    <th>IG</th>
                  </tr>
                </thead>
                <tbody>
                  {parsedResults.rows.map((row, i) => (
                    <tr key={i}>
                      <td>{row.keyIdx}</td>
                      <td>{row.ip}</td>
                      <td>{row.remark}</td>
                      <td className={row.status === 'OK' ? 'ok' : 'failed'}>
                        {row.status || 'FAILED'}
                      </td>
                      <td className={row.youtube === 'OK' ? 'ok' : 'failed'}>
                        {row.youtube || '-'}
                      </td>
                      <td className={row.instagram === 'OK' ? 'ok' : 'failed'}>
                        {row.instagram || '-'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function parseTestResults(str) {
  if (!str) return null;

  if (typeof str === 'object' && str.results) {
    return {
      okCount: str.ok || 0,
      totalCount: str.total || 0,
      rows: str.results.map(r => ({
        keyIdx: r.key_idx !== undefined ? r.key_idx : r.keyIdx,
        ip: r.ip || '',
        remark: r.remark || '',
        status: r.status || '',
        youtube: r.youtube || '-',
        instagram: r.instagram || '-',
      })),
    };
  }

  const lines = str.split('\n').filter(l => l.trim());
  const summaryLine = lines[0] || '';
  const m = summaryLine.match(/(\d+)\/(\d+)/);
  const okCount = m ? parseInt(m[1]) : 0;
  const totalCount = m ? parseInt(m[2]) : 0;
  const rows = lines.filter(l => l.startsWith('keyIdx:')).map(r => {
    const parts = r.split('|').map(s => s.trim());
    return {
      keyIdx: parts[0]?.replace(/^keyIdx:\s*/, '') || '',
      ip: parts[1] || '',
      remark: parts[2] || '',
      status: parts[3] || '',
      youtube: '-',
      instagram: '-',
    };
  });
  return { okCount, totalCount, rows };
}

// ─── Modal forms ───
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
  const [addInboundId, setAddInboundId] = React.useState(available[0]?.id || '');
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
          <input type="text" value={client.email} disabled style={{opacity: 0.6}} />
        </div>

        <div className="form-group">
          <label>Инбаунды</label>
          <div className="edit-inbound-list">
            {inbounds.map(inb => (
              <span key={inb} className="key-chip edit-inbound-chip">
                {inb}
                <button
                  className="edit-inbound-remove"
                  onClick={() => onDetachInbound(client.email, (allInbounds || []).find(ib => ib.remark === inb)?.id)}
                  title="Удалить инбаунд"
                >&times;</button>
              </span>
            ))}
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

export function AddSubscriptionModal({ onClose, onSubmit }) {
  const [name, setName] = React.useState('');

  const handleSubmit = (e) => {
    e.preventDefault();
    if (!name) return;
    onSubmit({ name, keys: [] });
  };

  return (
    <Modal title="📡 Новая подписка" onClose={onClose}>
      <form onSubmit={handleSubmit} className="modal-form">
        <div className="form-group">
          <label>Имя клиента</label>
          <input type="text" value={name} onChange={e => setName(e.target.value)} placeholder="ExampleClient" required autoFocus />
        </div>
        <p className="form-hint">Будет создан файл config-{name || '{ClientName}'}.txt в папке агрегатора</p>
        <div className="modal-actions">
          <button type="submit" className="btn btn-primary">📡 Создать</button>
          <button type="button" className="btn" onClick={onClose}>Отмена</button>
        </div>
      </form>
    </Modal>
  );
}
