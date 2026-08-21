import React from 'react';
import { Modal } from './Modal';

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
