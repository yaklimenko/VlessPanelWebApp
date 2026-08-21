import React from 'react';
import { Modal } from './Modal';

export function AddManualKSModal({ onClose, onSubmit }) {
  const [link, setLink] = React.useState('');
  const [label, setLabel] = React.useState('');
  const [error, setError] = React.useState('');

  const submit = () => {
    const l = link.trim();
    if (!l) { setError('Вставьте vless-ссылку'); return; }
    if (!l.startsWith('vless://')) { setError('Ссылка должна начинаться с vless://'); return; }
    onSubmit({ link: l, label: label.trim() || undefined });
  };

  return (
    <Modal title="🔗 Manual KeySource (свой ключ)" onClose={onClose}>
      <form onSubmit={(e) => { e.preventDefault(); submit(); }} className="modal-form">
        <div className="form-group">
          <label htmlFor="mksLink">vless-ссылка</label>
          <textarea id="mksLink" value={link} onChange={e => setLink(e.target.value)}
            placeholder="vless://uuid@host:port?..." rows="4" autoFocus
            style={{ fontFamily: 'monospace', fontSize: '12px' }} />
          {error && <div className="form-error">⚠ {error}</div>}
          <div className="form-hint">Вставляется как есть — REALITY/xhttp/tcp, любой транспорт</div>
        </div>
        <div className="form-group">
          <label htmlFor="mksLabel">Метка (опционально)</label>
          <input id="mksLabel" value={label} onChange={e => setLabel(e.target.value)}
            placeholder="Например: my-server-backup" maxLength="60"
            onKeyDown={e => { if (e.key === 'Enter') submit(); }} />
        </div>
        <div className="modal-actions">
          <button type="button" className="btn" onClick={onClose}>Отмена</button>
          <button type="submit" className="btn btn-primary">➕ Добавить</button>
        </div>
      </form>
    </Modal>
  );
}
