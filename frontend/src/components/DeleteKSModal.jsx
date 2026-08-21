import React from 'react';
import { Modal } from './Modal';

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
