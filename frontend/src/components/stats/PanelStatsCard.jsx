import { useEffect, useMemo, useState } from 'react';
import { api } from '../../api';
import { EChart } from './EChart';
import { fmtTomsk, fmtGB, kbPerSec, avgOf, maxOf, yMax } from './format';

const panelHost = (p) => { try { return new URL(p.url).hostname; } catch { return p.url || ''; } };

// Общие настройки tooltip/dataZoom — 1-в-1 с моком
const TOOLTIP = {
  trigger: 'axis',
  backgroundColor: '#161b22', borderColor: '#21262d',
  textStyle: { color: '#e6edf3', fontSize: 12 },
  axisPointer: {
    type: 'cross', lineStyle: { color: 'rgba(139,148,158,.4)' },
    crossStyle: { color: 'rgba(139,148,158,.5)' }, label: { backgroundColor: '#21262d' }
  }
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
const GRID = { left: 46, right: 14, top: 28, bottom: 36 };

function mkLineOpt(labels, series, yName, yMaxVal) {
  return {
    grid: GRID,
    tooltip: TOOLTIP,
    dataZoom: DATAZOOM,
    xAxis: {
      type: 'category', data: labels,
      axisLine: { lineStyle: { color: '#21262d' } },
      axisTick: { show: false },
      axisLabel: { color: '#8b949e', fontSize: 10, hideOverlap: true }
    },
    yAxis: {
      type: 'value', name: yName, max: yMaxVal,
      nameTextStyle: { color: '#8b949e', fontSize: 10 },
      axisLabel: { color: '#8b949e', fontSize: 10 },
      splitLine: { lineStyle: { color: 'rgba(33,38,45,.7)' } }
    },
    series
  };
}
function mkSeries(name, data, color, extra) {
  return Object.assign({
    name, type: 'line', data,
    smooth: true, symbol: 'none',
    lineStyle: { width: 1.5, color },
    itemStyle: { color }
  }, extra || {});
}

export function PanelStatsCard({ panel, range, availability }) {
  const [data, setData] = useState(null); // {points, bucketSeconds} | null
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    api.metricsSnapshots(panel.id, range)
      .then(res => { if (!cancelled) setData(res); })
      .catch(err => { if (!cancelled) setError(err.message || 'Ошибка загрузки'); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [panel.id, range]);

  const charts = useMemo(() => {
    if (!data || !data.points || data.points.length === 0) return null;
    const pts = data.points;
    const bucket = data.bucketSeconds || 300;
    const labels = pts.map(p => fmtTomsk(p.ts));
    const num = (v) => v == null ? null : v;

    const ramAvg = pts.map(p => num(p.memAvg));
    const ramMax = pts.map(p => num(p.memMax));
    const cpuAvg = pts.map(p => num(p.cpuAvg));
    const cpuMax = pts.map(p => num(p.cpuMax));
    const load1 = pts.map(p => num(p.load1Avg));
    const load5 = pts.map(p => num(p.load5Avg));
    const load15 = pts.map(p => num(p.load15Avg));
    const netUp = pts.map(p => kbPerSec(p.netUp, bucket));
    const netDown = pts.map(p => kbPerSec(p.netDown, bucket));
    const online = pts.map(p => num(p.onlineAvg));
    const conns = pts.map(p => num(p.openConnsMax));
    const last = pts[pts.length - 1];

    // RAM
    const ramChart = mkLineOpt(labels, [
      mkSeries('RAM avg %', ramAvg, '#58a6ff'),
      mkSeries('RAM max %', ramMax, '#d29922'),
    ], 'RAM %', yMax([...ramAvg, ...ramMax], 60));

    // CPU
    const cpuChart = mkLineOpt(labels, [
      mkSeries('CPU avg %', cpuAvg, '#3fb950'),
      mkSeries('CPU max %', cpuMax, '#f85149'),
    ], 'CPU %', yMax([...cpuAvg, ...cpuMax], 12));

    // Load
    const loadChart = mkLineOpt(labels, [
      mkSeries('load1', load1, '#58a6ff'),
      mkSeries('load5', load5, '#d29922'),
      mkSeries('load15', load15, '#8b949e'),
    ], 'load', yMax([...load1, ...load5, ...load15], 0.4));

    // Сеть (KB/s, area)
    const netChart = mkLineOpt(labels, [
      mkSeries('↑ net_up KB/s', netUp, '#58a6ff', { areaStyle: { opacity: .15 } }),
      mkSeries('↓ net_down KB/s', netDown, '#3fb950', { areaStyle: { opacity: .15 } }),
    ], 'KB/s', yMax([...netUp, ...netDown], 10));

    // Онлайн + open_conns (две оси)
    const connsChart = {
      grid: GRID,
      tooltip: TOOLTIP,
      dataZoom: DATAZOOM,
      legend: { top: 0, right: 4, textStyle: { color: '#8b949e', fontSize: 10 }, itemWidth: 12, itemHeight: 8 },
      xAxis: {
        type: 'category', data: labels,
        axisLine: { lineStyle: { color: '#21262d' } },
        axisTick: { show: false },
        axisLabel: { color: '#8b949e', fontSize: 10, hideOverlap: true }
      },
      yAxis: [
        { type: 'value', name: 'онлайн', min: 0, max: yMax(online, 5, 1.3), nameTextStyle: { color: '#8b949e', fontSize: 10 }, axisLabel: { color: '#8b949e', fontSize: 10 }, splitLine: { lineStyle: { color: 'rgba(33,38,45,.7)' } } },
        { type: 'value', name: 'conns', min: 0, max: yMax(conns, 100, 1.3), nameTextStyle: { color: '#8b949e', fontSize: 10 }, axisLabel: { color: '#8b949e', fontSize: 10 }, splitLine: { show: false } }
      ],
      series: [
        mkSeries('онлайн', online, '#58a6ff', { yAxisIndex: 0, lineStyle: { width: 1.5, color: '#58a6ff', type: 'dashed' } }),
        mkSeries('open_conns', conns, '#d29922', { yAxisIndex: 1 }),
      ]
    };

    // Диск — горизонтальный бар
    const du = last.diskUsed != null ? last.diskUsed / (1024 ** 3) : null;
    const dt = last.diskTotal != null ? last.diskTotal / (1024 ** 3) : null;
    const diskChart = dt != null && du != null ? {
      grid: { left: 12, right: 14, top: 14, bottom: 18 },
      tooltip: {
        trigger: 'item',
        backgroundColor: '#161b22', borderColor: '#21262d',
        textStyle: { color: '#e6edf3', fontSize: 12 },
        formatter: (pp) => pp.marker + ' ' + pp.seriesName + ': ' + pp.value + ' GB'
      },
      xAxis: { type: 'value', max: dt, show: false },
      yAxis: { type: 'category', data: [''], show: false },
      series: [
        { name: 'Занято', type: 'bar', stack: 'd', data: [+du.toFixed(2)], barWidth: 16, itemStyle: { color: '#58a6ff', borderRadius: [4, 0, 0, 4] } },
        { name: 'Свободно', type: 'bar', stack: 'd', data: [+(dt - du).toFixed(2)], itemStyle: { color: '#21262d', borderRadius: [0, 4, 4, 0] } }
      ]
    } : null;

    const lastXray = last.xrayOk === 1;
    return {
      labels, ramAvg, ramMax, cpuAvg, cpuMax, load1, load5, load15,
      netUp, netDown, online, conns, du, dt, last, lastXray,
      ramChart, cpuChart, loadChart, netChart, connsChart, diskChart,
    };
  }, [data]);

  // Недоступная панель: нет данных за выбранный диапазон
  if (!loading && (error || !charts)) {
    const lastSnap = availability && availability.lastSnapshotTs ? availability.lastSnapshotTs : null;
    const silence = lastSnap ? Math.floor((Date.now() / 1000 - lastSnap) / 3600) : null;
    return (
      <div className="panel-block unavailable">
        <div className="panel-block-head">
          <span className="panel-block-title">{panel.name}</span>
          <span className="panel-block-addr">{panelHost(panel)}</span>
          <span className="xray-badge err">🔴 error</span>
        </div>
        <div className="unavail-body">
          <span className="big-ico">⚠️</span>
          <div>
            <div className="warn-text"><b>Панель недоступна (нет телеметрии)</b> — API панели не отвечает, снапшоты не собираются.</div>
            <div className="last-seen">
              последний снапшот: {lastSnap ? fmtTomsk(lastSnap) : '—'}{silence != null ? ` · тишина: ${silence} ч` : ''}
            </div>
            {error && <div className="last-seen" style={{ color: 'var(--danger)' }}>{error}</div>}
          </div>
        </div>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="panel-block">
        <div className="panel-block-head">
          <span className="panel-block-title">{panel.name}</span>
          <span className="panel-block-addr">{panelHost(panel)}</span>
          <span className="snap-info"><span className="spin small" style={{ marginRight: 6 }} />загрузка…</span>
        </div>
      </div>
    );
  }

  const last = charts.last;
  const xrayOk = charts.lastXray;
  const onlineVal = last.onlineAvg != null ? last.onlineAvg : (last.onlineMax != null ? last.onlineMax : '—');

  return (
    <div className="panel-block">
      <div className="panel-block-head">
        <span className="panel-block-title">{panel.name}</span>
        <span className="panel-block-addr">{panelHost(panel)}</span>
        <span className={'xray-badge ' + (xrayOk ? 'ok' : 'err')}>
          {xrayOk ? '🟢 running' : '🔴 error'}
        </span>
        <span className="online-info">онлайн: <b>{onlineVal}</b></span>
        <span className="head-spacer"></span>
        <span className="snap-info">последний снапшот: {fmtTomsk(last.ts)}</span>
      </div>
      <div className="chart-grid">
        <div className="chart-box">
          <div className="chart-title">🧠 RAM <span className="val">avg {avgOf(charts.ramAvg) ?? '—'}% · max {maxOf(charts.ramMax) ?? '—'}%</span></div>
          <EChart option={charts.ramChart} />
        </div>
        <div className="chart-box">
          <div className="chart-title">⚡ CPU <span className="val">avg {avgOf(charts.cpuAvg) ?? '—'}% · max {maxOf(charts.cpuMax) ?? '—'}%</span></div>
          <EChart option={charts.cpuChart} />
        </div>
        <div className="chart-box">
          <div className="chart-title">📈 Load average <span className="val">1/5/15</span></div>
          <EChart option={charts.loadChart} />
        </div>
        <div className="chart-box">
          <div className="chart-title">🌐 Сеть <span className="val">KB/s</span></div>
          <EChart option={charts.netChart} />
        </div>
        <div className="chart-box">
          <div className="chart-title">👥 Онлайн + open_conns {!xrayOk && <span className="val">(xray down)</span>}</div>
          <EChart option={charts.connsChart} />
        </div>
        <div className="chart-box">
          <div className="chart-title">
            💾 Диск <span className="val">{charts.du != null && charts.dt != null
              ? `${charts.du.toFixed(2)} / ${charts.dt.toFixed(2)} GB (${Math.round(charts.du / charts.dt * 100)}%)` : '—'}</span>
          </div>
          {charts.diskChart ? <EChart option={charts.diskChart} /> : <div className="chart-empty">нет данных</div>}
        </div>
      </div>
    </div>
  );
}
