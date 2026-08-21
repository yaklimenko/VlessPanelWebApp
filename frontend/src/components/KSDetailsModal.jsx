import React from 'react';
import { Modal } from './Modal';
import { fmtDate, fmtBytes, fmtDateTime } from './format';

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
