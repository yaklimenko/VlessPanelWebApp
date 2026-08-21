import { fmtBytes, fmtShortDate, fmtDate } from './format';

export function KSChip({ subKey, keySource, onOpen, onCopy, onDelete, onTest, testing }) {
  const ks = keySource || null;

  // manual / legacy — grey chip
  if (!ks) {
    return (
      <div className="ks-chip ks-manual" title="manual · клик — детали" onClick={onOpen}>
        <span className="ks-dot manual"></span>
        <span className="ks-label">
          <span className="ks-server ell">manual</span>
          <span className="ks-sep">·</span>
          <span className="ks-inbound ell">{shortLink(subKey.link)}</span>
        </span>
        <span className="ks-meta">
          <span className="ks-status manual">manual</span>
          <button className="ks-ico js-copy" title="Скопировать ключ" onClick={(e) => { e.stopPropagation(); onCopy(subKey.link); }}>⧉</button>
          <button className="ks-ico del js-del" title="Удалить из подписки" onClick={(e) => { e.stopPropagation(); onDelete(subKey); }}>×</button>
        </span>
      </div>
    );
  }

  const st = ks.status || 'ok';
  const cls = 'ks-chip ' + (st === 'ok' ? '' : st === 'expired' ? 'ks-expired' : st === 'manual' ? 'ks-manual' : 'ks-missing');
  const statusTxt =
    st === 'ok' ? <span className="ks-status ok">ok</span>
    : st === 'expired' ? <span className="ks-status expired">закончился</span>
    : st === 'manual' ? <span className="ks-status manual">manual</span>
    : <span className="ks-status missing">{st === 'missing' ? 'missing' : 'панель недоступна'}</span>;

  let center = null;
  if (st === 'ok') {
    center = (
      <>
        <span className="ks-traffic" title="Трафик (clientStats 3X-UI)">
          ↑{fmtBytes(ks.traffic && ks.traffic.up)} ↓{fmtBytes(ks.traffic && ks.traffic.down)}
        </span>
        {ks.expireDate && <span className="ks-expiry" title="Окончание">до {fmtShortDate(ks.expireDate)}</span>}
      </>
    );
  } else if (st === 'expired') {
    center = (
      <>
        <svg className="ico-clock" width="14" height="14" viewBox="0 0 16 16" title={`Срок истёк ${fmtDate(ks.expireDate)}`}>
          <circle cx="8" cy="8" r="7" fill="none" stroke="currentColor" strokeWidth="1.6"/>
          <path d="M8 4.5V8l2.5 1.5" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"/>
        </svg>
        {ks.expireDate && <span className="ks-expiry">истёк {fmtShortDate(ks.expireDate)}</span>}
      </>
    );
  } else {
    center = (
      <>
        <svg className="ico-warn" width="14" height="14" viewBox="0 0 16 16">
          <path d="M8 1.5 15 14H1z" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round"/>
          <path d="M8 6v3.5" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"/>
          <circle cx="8" cy="12" r="0.9" fill="currentColor"/>
        </svg>
        <span className="ks-note">{ks.error || (st === 'missing' ? 'клиент/инбаунд не найден' : 'панель недоступна')}</span>
      </>
    );
  }

  const testRes = ks.lastTest;
  let testHtml;
  if (testing) {
    testHtml = <span className="ks-testres run"><span className="spin small"></span> тест…</span>;
  } else {
    let resTxt = '', resCls = '';
    if (testRes) {
      resCls = testRes.status === 'ok' ? 'ok' : 'fail';
      resTxt = testRes.status === 'ok'
        ? '✓ ' + (testRes.ms != null ? testRes.ms + ' мс' : 'ok')
        : '✗ ' + (testRes.error || 'ошибка');
    }
    testHtml = (
      <>
        <button className="ks-ico js-test" title="Тест ключа (test-single)" onClick={(e) => { e.stopPropagation(); onTest(ks); }}>🧪</button>
        {resTxt && <span className={`ks-testres ${resCls}`} title="Тест демоном vlesssubtest">{resTxt}</span>}
      </>
    );
  }

  return (
    <div className={cls} data-ks={ks.id} title="Клик — детали KeySource" onClick={onOpen}>
      <span className={`ks-dot ${st === 'manual' ? 'manual' : st === 'expired' ? 'expired' : st === 'ok' ? 'ok' : 'missing'}`}></span>
      <span className="ks-label">
        <span className="ks-server">{ks.panelName || '?'}</span>
        <span className="ks-sep">·</span>
        <span className="ks-inbound">{ks.inboundRemark || '—'}{ks.inboundPort ? ' :' + ks.inboundPort : ''}</span>
        <span className="ks-sep">·</span>
        <span className="ks-email">{ks.clientEmail}</span>
      </span>
      <span className="ks-meta">
        {statusTxt}
        {center}
        {testHtml}
        <button className="ks-ico js-copy" title="Скопировать vless-ключ" onClick={(e) => { e.stopPropagation(); onCopy(ks); }}>⧉</button>
        <button className="ks-ico del js-del" title="Удалить из подписки" onClick={(e) => { e.stopPropagation(); onDelete(subKey); }}>×</button>
      </span>
    </div>
  );
}

function shortLink(link) {
  if (!link) return '—';
  return link.length > 60 ? link.slice(0, 60) + '…' : link;
}
