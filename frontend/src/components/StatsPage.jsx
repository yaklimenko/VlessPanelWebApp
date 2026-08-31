import { useEffect, useState } from 'react';
import { api } from '../api';
import { PanelStatsCard } from './stats/PanelStatsCard';
import { StatsTestsTab } from './stats/StatsTestsTab';
import { fmtTomsk } from './stats/format';

const RANGES = [
  { key: '24h', label: '24h', sub: '24ч' },
  { key: '7d', label: '7d', sub: '7д' },
  { key: '90d', label: '90d', sub: '90д' },
];

export function StatsPage({ panels, subscriptions, showToast }) {
  const [tab, setTab] = useState('panels'); // 'panels' | 'tests'
  const [range, setRange] = useState('24h');
  const [availability, setAvailability] = useState({}); // panelId → {lastSnapshotTs}

  useEffect(() => {
    let cancelled = false;
    api.metricsAvailability()
      .then(list => {
        if (cancelled) return;
        const m = {};
        (list || []).forEach(a => { m[a.panelId] = a; });
        setAvailability(m);
      })
      .catch(() => { /* не критично */ });
    return () => { cancelled = true; };
  }, []);

  const rangeMeta = RANGES.find(r => r.key === range) || RANGES[0];

  return (
    <>
      <div className="page-tabs">
        <a className={'page-tab' + (tab === 'panels' ? ' active' : '')} onClick={(e) => { e.preventDefault(); setTab('panels'); }}>
          📈 Панели
        </a>
        <a className={'page-tab' + (tab === 'tests' ? ' active' : '')} onClick={(e) => { e.preventDefault(); setTab('tests'); }}>
          🧪 Тестирование ключей
        </a>
      </div>

      {tab === 'panels' ? (
        <div className="stats-body">
          <div className="page-head">
            <div className="page-title">
              Телеметрия панелей
              <span className="sub">снапшоты каждые 5 мин · {rangeMeta.sub}</span>
            </div>
            <div className="page-head-right">
              <span className="page-updated">обновлено: {fmtTomsk(Math.floor(Date.now() / 1000))} (Томск)</span>
              <div className="range-switch">
                {RANGES.map(r => (
                  <button
                    key={r.key}
                    className={'range-btn' + (range === r.key ? ' active' : '')}
                    onClick={() => setRange(r.key)}
                  >
                    {r.label}
                  </button>
                ))}
              </div>
            </div>
          </div>

          {panels.length === 0 ? (
            <div className="empty-state">
              <div className="icon">📭</div>
              <p>Нет панелей</p>
              <p className="hint">Добавьте панель на экране «Подписки», чтобы увидеть телеметрию</p>
            </div>
          ) : (
            panels.map(p => (
              <PanelStatsCard key={p.id} panel={p} range={range} availability={availability[p.id]} />
            ))
          )}
        </div>
      ) : (
        <StatsTestsTab subscriptions={subscriptions} showToast={showToast} />
      )}
    </>
  );
}
