import React from 'react';
import { Modal } from './Modal';

// Модалка переименования панели: одно поле «Название», prefilled текущим именем.
// Меняется только name — panelId/url/token не трогаем (кейсорцы и подписки
// ссылаются на панель по ID).
export function EditPanelNameModal({ panel, onClose, onSubmit }) {
  const [name, setName] = React.useState(panel?.name || '');

  const handleSubmit = (e) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    onSubmit(trimmed);
  };

  return (
    <Modal title="✏️ Переименовать панель" onClose={onClose}>
      <form onSubmit={handleSubmit} className="modal-form">
        <div className="form-group">
          <label>Название</label>
          <input
            type="text"
            value={name}
            onChange={e => setName(e.target.value)}
            placeholder="Hip Warsaw"
            required
            autoFocus
          />
        </div>
        <div className="modal-actions">
          <button type="submit" className="btn btn-primary">💾 Сохранить</button>
          <button type="button" className="btn" onClick={onClose}>Отмена</button>
        </div>
      </form>
    </Modal>
  );
}
