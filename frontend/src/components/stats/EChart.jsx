import { useEffect, useRef } from 'react';
import * as echarts from 'echarts';

// Тонкая обёртка над ECharts: инициализация/диспоуз + setOption(notMerge).
// Один экземпляр на контейнер, resize на window.resize.
export function EChart({ option, className, style }) {
  const elRef = useRef(null);
  const chartRef = useRef(null);

  useEffect(() => {
    if (!elRef.current) return;
    const chart = echarts.init(elRef.current);
    chartRef.current = chart;
    const onResize = () => chart.resize();
    window.addEventListener('resize', onResize);
    return () => {
      window.removeEventListener('resize', onResize);
      chart.dispose();
      chartRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (chartRef.current && option) {
      chartRef.current.setOption(option, true);
    }
  }, [option]);

  return <div ref={elRef} className={className || 'chart'} style={style} />;
}
