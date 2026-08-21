import React from 'react';
import { Modal } from './Modal';

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
