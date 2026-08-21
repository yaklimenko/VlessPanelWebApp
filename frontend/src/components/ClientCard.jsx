import { fmtBytes, fmtShortDate, fmtDate } from './format';

export function ClientCard({ client, inbounds, keySources, activeSubKeys, onChipClick, onOpenClient, panelName, panelId }) {
  const chipInbounds = (inbounds || []).filter(ib => (client.inboundIds || []).includes(ib.id));

  const statusFor = (inboundId) => {
    const ks = (keySources || []).find(k =>
      k.type === 'panel' && k.panelId === panelId && k.clientEmail === client.email && k.inboundId === inboundId);
    return ks || null;
  };

  const expiryMs = client.expiryTime || 0;
  const expiryStr = expiryMs > 0 ? ' · до ' + fmtShortDate(expiryMs) : '';
  const trafficStr = (client.up || client.down) ? ` · ↑${fmtBytes(client.up)} ↓${fmtBytes(client.down)}` : '';

  return (
    <div className="client-card" onClick={() => onOpenClient && onOpenClient(client)}>
      <div className="client-top">
        <div>
          <div className="client-name">
            {client.email}
            {client.enable ? <span className="badge ok">вкл</span> : <span className="badge">выкл</span>}
          </div>
          <div className="client-inbounds">
            {chipInbounds.length} {chipInbounds.length === 1 ? 'инбаунд' : (chipInbounds.length < 5 ? 'инбаунда' : 'инбаундов')}
            {expiryStr}{trafficStr}
          </div>
        </div>
      </div>
      <div className="client-chips">
        {chipInbounds.length === 0 ? (
          <div className="client-noinb">нет инбаундов — привяжите на панели</div>
        ) : chipInbounds.map(ib => {
          const ks = statusFor(ib.id);
          const st = ks ? ks.status : 'ok'; // без KeySource — статус неизвестен, показываем как ok
          const added = !!(ks && activeSubKeys && activeSubKeys.has(ks.id));
          const inactive = ib.enable === false;
          return (
            <button
              key={ib.id}
              className={`inb-chip${added ? ' added' : ''}${inactive ? ' inactive' : ''}`}
              title={`${panelName} · ${ib.remark} :${ib.port} · ${client.email}${inactive ? ' · ⚠️ инбаунд неактивен' : ''}${ks && ks.expireDate ? ' · до ' + fmtDate(ks.expireDate) : ''}${added ? '\nуже добавлено в подписку' : ''}`}
              onClick={(e) => { e.stopPropagation(); onChipClick && onChipClick(client, ib); }}
            >
              <span className={`idot ${st}${inactive ? ' inactive' : ''}`}></span>
              <span className="inb-name">{ib.remark}</span>
              <span className="inb-port">:{ib.port}</span>
              {added && <span className="inb-ok">✓</span>}
            </button>
          );
        })}
      </div>
    </div>
  );
}
