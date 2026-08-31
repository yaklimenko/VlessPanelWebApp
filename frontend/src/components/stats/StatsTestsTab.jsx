import { useEffect, useMemo, useState } from 'react';
import { api } from '../../api';
import { EChart } from './EChart';
import { fmtTomskSmart, fmtDur, avgOf, fmtNum4 } from './format';

const TOOLTIP = {
  trigger: 'axis',
  backgroundColor: '#161b22', borderColor: '#21262d',
  textStyle: { color: '#e6edf3', fontSize: 12 },
  axisPointer: { type: 'cross', lineStyle: { color: 'rgba(139,148,158,.4)' }, label: { backgroundColor: '#21262d' } }
};
const DATAZOOM = [
  { type: 'inside' },
  {
    type: 'slider', height: 14, bottom: 4,
    borderColor: '#21262d', backgroundColor: 'rgba(22,27,34,.7)',
    fillerColor: 'rgba(88,166,255,.18)',
    handleStyle: { color: '#58a6ff' }, moveHandleStyle: { color: '#21262d' },
    textStyle: { color: '#8b949e', fontSize: 10 },
    dataBackground: { lineStyle: { color: '#30363d' }, areaStyle: { color: 'rgba(48,54,61,.3)' } }
  }
];

const STATUS_TEXT = {
  ok: { txt: '✅ ok', cls: 'st-ok' },
  partial: { txt: '🟡 partial', cls: 'st-partial' },
  failed: { txt: '❌ failed', cls: 'st-fail' },
  running: { txt: '⏳ running', cls: 'st-partial' },
};
const KEY_ICON = { OK: '✅', FAIL: '❌', TIMEOUT: '❌', ERROR: '❌', DEGRADED: '⚠️' };

// Ключ «деградировал»: частичный отказ (status = DEGRADED). YT/IG в крон-прогонах
// больше нет (speed-тест через probe_url) — смотрим только на статус.
const isDegraded = (k) => k.status === 'DEGRADED';

export function StatsTestsTab({ subscriptions }) {
  const [runs, setRuns] = useState(null);      // список прогонов (7д)
  const [error, setError] = useState(null);
  const [details, setDetails] = useState({});   // runId → results
  const [loadingDetails, setLoadingDetails] = useState({});
  const [openRunId, setOpenRunId] = useState(null);
  const [filterSub, setFilterSub] = useState('all');
  const [filterStatus, setFilterStatus] = useState('all');

  useEffect(() => {
    let cancelled = false;
    api.metricsTestRuns('7d')
      .then(res => { if (!cancelled) setRuns(res.runs || []); })
      .catch(err => { if (!cancelled) setError(err.message || 'Ошибка загрузки'); });
    return () => { cancelled = true; };
  }, []);

  const subNameById = useMemo(() => {
    const m = {};
    (subscriptions || []).forEach(s => { m[s.id] = s.name; });
    return m;
  }, [subscriptions]);

  // Подписки, встречающиеся в прогонах (для фильтра)
  const subOptions = useMemo(() => {
    if (!runs) return [];
    const ids = [...new Set(runs.map(r => r.subscriptionId).filter(Boolean))];
    return ids.map(id => ({ id, name: subNameById[id] || id }));
  }, [runs, subNameById]);

  // Сводки
  const summary = useMemo(() => {
    if (!runs || runs.length === 0) return null;
    const total = runs.length;
    const sumKeys = runs.reduce((s, r) => s + (r.total || 0), 0);
    const sumOk = runs.reduce((s, r) => s + (r.okCount || 0), 0);
    const sumFail = runs.reduce((s, r) => s + (r.failCount || 0), 0);
    // DEGRADED — из загруженных деталей (частичный отказ)
    let sumDeg = 0;
    Object.values(details).forEach(keys => {
      (keys || []).forEach(k => { if (isDegraded(k)) sumDeg++; });
    });
    return {
      total,
      okPct: sumKeys > 0 ? Math.round(sumOk / sumKeys * 100) : null,
      sumFail,
      sumDeg,
      firstTs: runs[runs.length - 1]?.startedAt,
      lastTs: runs[0]?.startedAt,
    };
  }, [runs, details]);

  // Тренд ok%
  const trendOption = useMemo(() => {
    if (!runs || runs.length === 0) return null;
    const labels = runs.map(r => fmtTomskSmart(Date.parse(r.startedAt) / 1000));
    const data = runs.map(r => r.total > 0 ? +(r.okCount / r.total * 100).toFixed(1) : null);
    const avgOk = avgOf(data);
    return {
      grid: { left: 46, right: 20, top: 30, bottom: 40 },
      tooltip: {
        ...TOOLTIP,
        formatter: (ps) => {
          const p = ps[0];
          const run = runs[p.dataIndex];
          return p.name + '<br/><b style="color:#58a6ff">ok%: ' + p.value + '%</b><br/>' +
            run.total + ' ключей · ' + run.okCount + ' OK · ' + run.failCount + ' FAIL' +
            '<br/>статус: ' + run.status;
        }
      },
      dataZoom: DATAZOOM,
      xAxis: {
        type: 'category', data: labels,
        axisLine: { lineStyle: { color: '#21262d' } },
        axisTick: { show: false },
        axisLabel: { color: '#8b949e', fontSize: 10, hideOverlap: true }
      },
      yAxis: {
        type: 'value', name: 'ok%', min: 50, max: 100,
        nameTextStyle: { color: '#8b949e', fontSize: 10 },
        axisLabel: { color: '#8b949e', fontSize: 10 },
        splitLine: { lineStyle: { color: 'rgba(33,38,45,.7)' } }
      },
      series: [{
        name: 'ok%', type: 'line', data,
        smooth: true, symbol: 'none',
        lineStyle: { width: 2, color: '#58a6ff' },
        itemStyle: { color: '#58a6ff' },
        areaStyle: { color: 'rgba(88,166,255,.12)' },
        markLine: avgOk != null ? {
          silent: true, symbol: 'none',
          lineStyle: { color: '#d29922', type: 'dashed', width: 1 },
          label: { formatter: 'avg ' + avgOk + '%', color: '#d29922', fontSize: 10, position: 'insideEndTop' },
          data: [{ yAxis: avgOk }]
        } : undefined
      }]
    };
  }, [runs]);

  // Фильтрация журнала
  const filteredRuns = useMemo(() => {
    if (!runs) return [];
    return runs.filter(r => {
      if (filterSub !== 'all' && r.subscriptionId !== filterSub) return false;
      if (filterStatus !== 'all' && r.status !== filterStatus) return false;
      return true;
    });
  }, [runs, filterSub, filterStatus]);

  // Загрузка деталей прогона (по клику, кэш в details)
  const loadDetail = async (runId) => {
    if (details[runId]) return;
    setLoadingDetails(prev => ({ ...prev, [runId]: true }));
    try {
      const res = await api.metricsTestRunDetail(runId);
      setDetails(prev => ({ ...prev, [runId]: res.results || [] }));
    } catch { /* молча — покажем «не удалось загрузить» */ }
    finally { setLoadingDetails(prev => ({ ...prev, [runId]: false })); }
  };

  const toggleRun = (runId) => {
    if (openRunId === runId) { setOpenRunId(null); return; }
    setOpenRunId(runId);
    loadDetail(runId);
  };

  if (error && !runs) {
    return (
      <div className="stats-body">
        <div className="empty-state">
          <div className="icon">⚠️</div>
          <p>Не удалось загрузить прогоны тестов</p>
          <p className="hint">{error}</p>
        </div>
      </div>
    );
  }

  if (!runs) {
    return (
      <div className="stats-body">
        <div className="loading-state"><div className="spin"></div><p>Загружаем прогоны тестов…</p></div>
      </div>
    );
  }

  return (
    <div className="stats-body">
      <div className="page-head">
        <div className="page-title">
          Тестирование ключей
          <span className="sub">автопрогон каждые 4ч · за 7 дней</span>
        </div>
        <div className="page-head-right">
          <div className="filters">
            <select value={filterSub} onChange={e => setFilterSub(e.target.value)}>
              <option value="all">Все подписки</option>
              {subOptions.map(o => <option key={o.id} value={o.id}>{o.name}</option>)}
            </select>
            <select value={filterStatus} onChange={e => setFilterStatus(e.target.value)}>
              <option value="all">Все статусы</option>
              <option value="ok">ok</option>
              <option value="partial">partial</option>
              <option value="failed">failed</option>
              <option value="running">running</option>
            </select>
          </div>
        </div>
      </div>

      {/* Сводные карточки */}
      <div className="summary-cards">
        <div className="sum-card">
          <div className="num accent">{summary ? summary.total : '—'}</div>
          <div className="lbl">Всего прогонов</div>
          <div className="hint">за 7 дней</div>
        </div>
        <div className="sum-card">
          <div className="num">{summary && summary.okPct != null ? `~${summary.okPct}%` : '—'}</div>
          <div className="lbl">Средний ok%</div>
          <div className="hint">по всем ключам прогонов</div>
        </div>
        <div className="sum-card">
          <div className="num danger">{summary ? summary.sumFail : '—'}</div>
          <div className="lbl">FAIL ключей (7д)</div>
          <div className="hint">сумма failCount по прогонам</div>
        </div>
        <div className="sum-card">
          <div className="num warn">{summary ? summary.sumDeg : '—'}</div>
          <div className="lbl">DEGRADED ключей</div>
          <div className="hint">частичный отказ (YT/IG)</div>
        </div>
      </div>

      {/* Тренд ok% */}
      <div className="trend-box">
        <div className="trend-head">
          <h3>Тренд ok% по прогонам</h3>
          <span className="hint">
            {summary && summary.firstTs
              ? `видна деградация во времени · ${summary.total} прогонов · ${fmtTomskSmart(Date.parse(summary.firstTs) / 1000)} — ${fmtTomskSmart(Date.parse(summary.lastTs) / 1000)}`
              : 'данных пока нет'}
          </span>
        </div>
        {trendOption ? <EChart option={trendOption} /> : <div className="chart-empty" style={{ height: 200 }}>нет данных</div>}
      </div>

      {/* Журнал прогонов */}
      <div className="log-box">
        <div className="log-head">
          <h3>Журнал прогонов</h3>
          <span className="hint">клик по строке — детали по ключам</span>
        </div>
        {filteredRuns.length === 0 ? (
          <div className="empty-state"><div className="icon">📭</div><p>Нет прогонов по выбранным фильтрам</p></div>
        ) : (
          <table className="run-table">
            <thead>
              <tr>
                <th>Время (Томск)</th>
                <th>Подписка</th>
                <th>Статус</th>
                <th>Всего</th>
                <th>OK</th>
                <th>FAIL</th>
                <th>Длительность</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {filteredRuns.map(run => {
                const st = STATUS_TEXT[run.status] || { txt: run.status, cls: 'st-fail' };
                const started = Date.parse(run.startedAt) / 1000;
                const finished = run.finishedAt ? Date.parse(run.finishedAt) / 1000 : null;
                const dur = finished && started ? finished - started : null;
                const isOpen = openRunId === run.id;
                const keys = details[run.id];
                const loadingDet = loadingDetails[run.id];
                return [
                  <tr key={'r' + run.id} className={'run-row' + (isOpen ? ' open' : '')} onClick={() => toggleRun(run.id)}>
                    <td className="t-time">{fmtTomskSmart(started)}</td>
                    <td>{subNameById[run.subscriptionId] || run.subscriptionId || '—'}</td>
                    <td><span className={st.cls}>{st.txt}</span></td>
                    <td>{run.total}</td>
                    <td className="c-ok">{run.okCount}</td>
                    <td className="c-fail">{run.failCount}</td>
                    <td className="t-dur">{dur != null ? fmtDur(dur) : '—'}</td>
                    <td className="t-chev">▶</td>
                  </tr>,
                  <tr key={'d' + run.id} className="detail-row" style={{ display: isOpen ? '' : 'none' }}>
                    <td colSpan={8}>
                      <div className="detail-inner">
                        <div className="detail-head">
                          <span>Ключи прогона · <b>{fmtTomskSmart(started)}</b></span>
                          <span><b className="c-ok">{run.okCount}</b> OK</span>
                          <span><b className="c-fail">{run.failCount}</b> FAIL</span>
                          {dur != null && <span>длительность: <b>{fmtDur(dur)}</b></span>}
                        </div>
                        {loadingDet && !keys ? (
                          <div className="loading-state" style={{ padding: 12 }}><div className="spin small"></div><p style={{ fontSize: 12 }}>Загружаем результаты…</p></div>
                        ) : !keys ? (
                          <div className="empty-state" style={{ padding: 12 }}><p style={{ fontSize: 12 }}>Не удалось загрузить результаты</p></div>
                        ) : keys.length === 0 ? (
                          <div className="empty-state" style={{ padding: 12 }}><p style={{ fontSize: 12 }}>Нет результатов по ключам</p></div>
                        ) : (
                          <div className="kr-table-wrap">
                            <table className="kr-table">
                              <thead>
                                <tr>
                                  <th className="kr-th-st" title="Статус">Статус</th>
                                  <th className="kr-th-label" title="Ключ">Ключ</th>
                                  <th className="kr-th-ip" title="IP сервера">IP</th>
                                  <th className="kr-th-num" title="Средняя скорость (Мбит/с)">⚡ Скорость</th>
                                  <th className="kr-th-num" title="Стабильность соединения (%)">Стабильность</th>
                                  <th className="kr-th-num" title="Успешные сессии / всего">Сессии</th>
                                  <th className="kr-th-num" title="Переподключения">Реконнекты</th>
                                  <th className="kr-th-num" title="Задержка (мс)">⏱ Latency</th>
                                  <th className="kr-th-num" title="Скачано (MB)">Скачано</th>
                                  <th className="kr-th-num" title="Длительность теста">Время</th>
                                </tr>
                              </thead>
                              <tbody>
                                {keys.map(k => {
                                  const icon = KEY_ICON[k.status] || '❔';
                                  const warn = isDegraded(k);
                                  const speed = k.avgSpeedKbps != null && k.avgSpeedKbps > 0
                                    ? (k.avgSpeedKbps / 1000).toFixed(1) + ' Мбит/с' : '—';
                                  const stab = k.stabilityPct != null && k.stabilityPct > 0
                                    ? fmtNum4(k.stabilityPct) + '%' : '—';
                                  const sessOk = k.sessionsOk != null ? k.sessionsOk : 0;
                                  const sessFail = k.sessionsFail != null ? k.sessionsFail : 0;
                                  const sessions = (k.sessionsOk != null || k.sessionsFail != null)
                                    ? sessOk + '/' + (sessOk + sessFail) : '—';
                                  const reconn = k.reconnects != null && k.reconnects > 0 ? k.reconnects : '—';
                                  const lat = k.latencyMs != null ? Math.round(k.latencyMs) + ' ms' : '—';
                                  const dl = k.totalDownloadedMb != null && k.totalDownloadedMb > 0
                                    ? fmtNum4(k.totalDownloadedMb) + ' MB' : '—';
                                  const dur = k.durationSec != null && k.durationSec > 0
                                    ? fmtDur(k.durationSec) : '—';
                                  return (
                                    <tr key={k.id} className={'kr-tr' + (warn ? ' warn' : '')}>
                                      <td className="kr-st" title={k.status}>{icon}</td>
                                      <td className="kr-label" title={k.label}>{k.label}</td>
                                      <td className="kr-ip">{k.ip || '—'}</td>
                                      <td className="kr-num">{speed}</td>
                                      <td className="kr-num">{stab}</td>
                                      <td className="kr-num">{sessions}</td>
                                      <td className="kr-num">{reconn}</td>
                                      <td className={'kr-num' + (warn ? ' warn' : '')}>{lat}</td>
                                      <td className="kr-num">{dl}</td>
                                      <td className="kr-num">{dur}</td>
                                    </tr>
                                  );
                                })}
                              </tbody>
                            </table>
                          </div>
                        )}
                      </div>
                    </td>
                  </tr>,
                ];
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
