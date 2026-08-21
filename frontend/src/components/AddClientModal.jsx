import React from 'react';
import { Modal } from './Modal';

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
