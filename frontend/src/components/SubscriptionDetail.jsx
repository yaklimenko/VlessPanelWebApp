import React from 'react';
import { KSChip } from './KSChip';
import { fmtDateTime } from './format';
import { getPublicUrl } from '../api';

export function SubscriptionDetail({
  sub, keySourceById, testingKs, generating, testingSub, testResults, testableCount,
  onGenerate, onTestSub, onDelete,
  onCopyKey, onCopyKeyFromChip, onRemoveKey, onTestKS, onOpenKS, onRefresh,
  onAddManual, onCopyValue,
}) {
  const statusBadge = sub.status === 'active'
    ? <span className="badge ok">● включен</span>
    : <span className="badge">○ черновик</span>;

  let syncBadge;
  if (sub.status !== 'active') {
    syncBadge = <span className="badge">файл не создан</span>;
  } else if (sub.synced === true) {
    syncBadge = <span className="badge ok">✓ синхронизировано</span>;
  } else if (sub.synced === false) {
    syncBadge = <span className="badge warn">⚠ изменено — требуется синк</span>;
  } else {
    syncBadge = <span className="badge">— статус синка неизвестен</span>;
  }

  const hasKeys = (sub.keys || []).length > 0;
  const genLabel = sub.status === 'active' ? '🔄 Перегенерировать' : '🚀 Создать подписку';

  return (
    <>
      <div className="sub-detail-header">
        <div className="sub-detail-title">
          <h3>{sub.name}</h3>
          {statusBadge}{syncBadge}
          <span className="sub-updated">изменена: {fmtDateTime(sub.updatedAt)}</span>
        </div>
        <div className="sub-detail-sub">
          🔑 {hasKeys ? sub.keys.length : 0} {(sub.keys || []).length === 1 ? 'ключ' : ((sub.keys || []).length < 5 ? 'ключа' : 'ключей')} · порядок = порядок добавления
        </div>
        <div className="sub-actions">
          <button className="btn btn-success" onClick={onGenerate} disabled={!hasKeys || generating}>
            {generating ? <span className="spin"></span> : genLabel}
          </button>
          <button className="btn" onClick={onTestSub} disabled={!testableCount || testingSub} title={testingSub ? 'Тест идёт…' : 'Тест подписки'}>
            {testingSub ? <span className="spin small"></span> : '🧪'} Тест подписки
          </button>
          <button className="btn btn-sm" onClick={onAddManual} title="Добавить свой vless-ключ вручную"><span className="ks-dot manual" style={{ display: 'inline-block', marginRight: 6, verticalAlign: 'middle' }}></span>+ manual</button>
          <button className="btn btn-sm" onClick={onRefresh} title="Обновить статусы">🔄</button>
          <button className="btn btn-sm btn-danger" onClick={onDelete} title="Удалить подписку">🗑</button>
        </div>
      </div>

      <div className="sub-detail-body">
        <div className="keys-area">
          {(sub.keys || []).length === 0 ? (
            <div className="empty-state" style={{ padding: 22 }}>
              <div className="icon">🧩</div>
              <p>Подписка пуста</p>
              <p className="hint">Кликайте по чипсам инбаундов слева — KeySource будет добавляться сюда</p>
            </div>
          ) : (
            (sub.keys || []).map(k => (
              <KSChip
                key={k.id}
                subKey={k}
                keySource={k.keySourceId ? (keySourceById[k.keySourceId] || null) : null}
                testing={testingKs === k.keySourceId}
                onOpen={() => onOpenKS(k.keySourceId ? (keySourceById[k.keySourceId] || null) : null, k)}
                onCopy={onCopyKeyFromChip}
                onDelete={(subKey) => onRemoveKey(subKey)}
                onTest={(ks) => onTestKS(ks)}
              />
            ))
          )}
        </div>

        <div className="test-block">
          <div className="test-block-head">
            <h4>🧪 Тестирование</h4>
            <span className="hint-inline">этап 1 — тест каждого KeySource (test-single) · этап 2 — тест подписки перед синком</span>
          </div>
          <div className="form-hint" style={{ marginBottom: 6 }}>
            Тест каждого ключа — кнопка <b>🧪</b> на чипсе или в модалке деталей. «Тест подписки» гоняет все ключи через демон vlesssubtest.
          </div>
          {testResults && <TestResultsTable data={testResults} />}
        </div>

        <div className="file-info">
          {sub.status === 'active' ? (
            <>
              <div className="row">
                <span>Файл:</span><code className="copyable" title="Клик — скопировать" onClick={() => onCopyValue(`configs-${sub.name}.txt`, 'Имя файла')}>configs-{sub.name}.txt</code>
                <span>·</span><span>Ссылка:</span><code className="copyable" title="Клик — скопировать" onClick={() => onCopyValue(`${getPublicUrl()}/sub/${sub.name}`, 'Ссылка подписки')}>{getPublicUrl()}/sub/{sub.name}</code>
              </div>
              <div className="row">
                <span>Локально (mtime):</span><code>{fmtDateTime(sub.fileMtime) || '—'}</code>
                {sub.synced === true && <span className="sync-ok">— синхронизировано</span>}
                {sub.synced === false && <span className="sync-warn">— есть изменения, нужен синк</span>}
              </div>
            </>
          ) : (
            <div className="row">
              <span className="sync-warn">Файл не создан — нажмите «Создать подписку», когда добавлены чипсы</span>
            </div>
          )}
        </div>
      </div>
    </>
  );
}

function TestResultsTable({ data }) {
  const rows = (data.results || []).map((r, i) => ({
    idx: i,
    ip: r.ip || '',
    remark: r.remark || '',
    status: r.status || '',
    youtube: r.youtube || '-',
    instagram: r.instagram || '-',
  }));
  return (
    <div className="test-results">
      <div className="test-summary">
        <span>Результат: <b className="ok">{data.ok} OK</b> / <b className="fail">{data.total - data.ok} не прошли</b></span>
      </div>
      <table className="test-table">
        <thead>
          <tr><th>#</th><th>IP</th><th>Remarks</th><th>Status</th><th>YT</th><th>IG</th></tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i}>
              <td>{row.idx}</td>
              <td>{row.ip}</td>
              <td>{row.remark}</td>
              <td className={row.status === 'OK' ? 'ok' : 'fail'}>{row.status || 'FAILED'}</td>
              <td className={row.youtube.startsWith('OK') ? 'ok' : 'fail'}>{row.youtube}</td>
              <td className={row.instagram.startsWith('OK') ? 'ok' : 'fail'}>{row.instagram}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
