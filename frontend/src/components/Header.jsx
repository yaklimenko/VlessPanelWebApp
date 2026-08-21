export function Header({ panels, selectedPanelId, onPanelChange, onAddPanel, onDeletePanel, onSyncAll, syncing, onRegenerateAll, regenerating }) {
  return (
    <header className="header">
      <div className="header-left">
        <span className="app-title">Vless<span className="accent">Panel</span></span>
        <button className="btn btn-danger btn-sm" disabled={panels.length === 0} onClick={onDeletePanel}>– Панель</button>
        <button className="btn btn-primary btn-sm" onClick={onAddPanel}>+ Панель</button>
      </div>
      <div className="header-right">
        <button className="btn btn-sm" onClick={onRegenerateAll} disabled={regenerating} title="Свежие ключи с панелей для всех подписок с panel-KeySource">
          {regenerating ? <span className="spin small"></span> : '🔄'} Перегенерировать все
        </button>
        <button className="btn btn-sm btn-success" onClick={onSyncAll} disabled={syncing}>
          {syncing ? '⟳ Синхронизация…' : '⟳ Синк с агрегатором'}
        </button>
        <span className="app-version">v0.9.0</span>
      </div>
    </header>
  );
}
