// Сайдбар (Этап 3) — переключение между «Подписки» и «Статистика».
// Без react-router: активный пункт и переключение — через props из App.
export function Sidebar({ activeView, onNavigate }) {
  return (
    <aside className="sidebar">
      <div className="side-logo">Vless<span className="accent">Panel</span></div>
      <nav className="side-nav">
        <a
          className={'side-link' + (activeView === 'subscriptions' ? ' active' : '')}
          onClick={(e) => { e.preventDefault(); onNavigate('subscriptions'); }}
          title="Подписки"
        >
          <span className="ico">🔑</span><span className="txt">Подписки</span>
        </a>
        <a
          className={'side-link' + (activeView === 'stats' ? ' active' : '')}
          onClick={(e) => { e.preventDefault(); onNavigate('stats'); }}
          title="Статистика"
        >
          <span className="ico">📊</span><span className="txt">Статистика</span>
        </a>
      </nav>
      <div className="side-foot">v0.9.0 · Этап 3</div>
    </aside>
  );
}
