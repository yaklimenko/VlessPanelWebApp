import React from 'react';
import { Modal } from './Modal';
import { getPublicUrl } from '../api';

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
          Файл: <code className="mono">configs-{subName}.txt</code> · ссылка: <code className="mono">{getPublicUrl()}/sub/{subName}</code>
          <br />Не забудьте синхронизировать с агрегатором.
        </div>
        <div className="modal-actions">
          <button className="btn btn-primary" onClick={onClose}>Понятно</button>
        </div>
      </div>
    </Modal>
  );
}
