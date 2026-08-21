import React, { useState } from 'react';
import { Modal } from './Modal';

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
