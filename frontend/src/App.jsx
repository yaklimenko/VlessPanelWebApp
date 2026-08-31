import React, { useState } from 'react';
import { useVlessPanel } from './hooks/useVlessPanel';
import {
  ToastProvider,
  Header, Sidebar, ClientCard, StatsPage,
  NewSubModal, KSDetailsModal, DeleteSubModal, DeleteKSModal, ReportModal,
  AddPanelModal, AddClientModal, AddManualKSModal, EditClientModal,
} from './components';
import { SubscriptionDetail } from './components/SubscriptionDetail';
import { AuthGate } from './components/AuthGate';

const panelHost = (p) => { try { return new URL(p.url).hostname; } catch { return p.url || ''; } };

function AppInner() {
  const [activeView, setActiveView] = useState(() => {
    try { return localStorage.getItem('vlesspanel:view') === 'stats' ? 'stats' : 'subscriptions'; } catch { return 'subscriptions'; }
  });
  const navigate = (v) => { setActiveView(v); try { localStorage.setItem('vlesspanel:view', v); } catch {} };
  const {
    panels, currentPanelId, setCurrentPanelId, clients, inbounds, clientsError,
    clientSearch, setClientSearch, loadingClients,
    showAddPanel, setShowAddPanel, showAddClient, setShowAddClient,
    showAddManualKS, setShowAddManualKS, editingClient, setEditingClient,
    keySources, testingKs, subscriptions, activeSubId, setActiveSubId,
    generating, testingSubs, subTestResults, syncing,
    showNewSub, setShowNewSub, pendingKS, setPendingKS,
    ksDetails, setKsDetails, deleteSub, setDeleteSub, deleteKS, setDeleteKS,
    report, setReport, regenerating,
    activeSub, activeSubKeys, panel, filteredClients, sortedPanels, ksCountByPanel,
    keySourceById, sortedSubs, testableCount,
    loadData, showToast,
    handleAddPanel, handleDeletePanel, handleCreateClient, handleAttachInbound,
    handleDetachInbound, handleUpdateClient, copyToClipboard, handleChipClick,
    handleAddManualKS, handleNewSub, handleGenerate, handleRemoveKey, copyKSKey,
    handleTestKS, handleTestSub, handleDeleteSub, handleDeleteKS,
    handleRegenerateAll, handleSyncAll,
  } = useVlessPanel();

  return (
    <div className="app">
      <Sidebar activeView={activeView} onNavigate={navigate} />

      <div className="main-wrap">
        <Header
          panels={panels}
          selectedPanelId={currentPanelId}
          onPanelChange={setCurrentPanelId}
          onAddPanel={() => setShowAddPanel(true)}
          onDeletePanel={handleDeletePanel}
          onSyncAll={handleSyncAll}
          syncing={syncing}
          onRegenerateAll={handleRegenerateAll}
          regenerating={regenerating}
          activeView={activeView}
        />

        {activeView === 'stats' ? (
          <StatsPage panels={panels} subscriptions={subscriptions} showToast={showToast} />
        ) : (
          <div className="main">
        {/* ─── Left: panels → clients → inbound chips ─── */}
        <section className="col">
          <div className="col-header">
            <h2>🔑 Источники ключей{panel && <span className="panel-sub"> · {panel.name} ({panelHost(panel)})</span>}</h2>
            <div className="right">
              <span className="badge accent">
                {(keySources || []).length} {(keySources || []).length === 1 ? 'источник' : 'источников'}
              </span>
              <button className="btn btn-sm" title="Добавить клиента" onClick={() => { if (!currentPanelId) { showToast('⚠️ Сначала выберите панель'); return; } setShowAddClient(true); }}>👤 + клиент</button>
            </div>
          </div>
          {panels.length > 0 && (
            <div className="column-tools">
              <select className="panel-select" value={currentPanelId || ''} onChange={e => setCurrentPanelId(e.target.value)}>
                {sortedPanels.map(p => {
                  const cnt = ksCountByPanel[p.id] || 0;
                  const unreachable = (keySources || []).some(ks => ks.type === 'panel' && ks.panelId === p.id && ks.status === 'panel_unreachable');
                  return (
                    <option key={p.id} value={p.id}>
                      {unreachable ? '○' : '●'} {p.name} ({panelHost(p)}) · {cnt}
                    </option>
                  );
                })}
              </select>

              <div className="search-wrap">
                <span className="search-icon">🔍</span>
                <input type="text" placeholder="Поиск клиентов и инбаундов…"
                  value={clientSearch} onChange={e => setClientSearch(e.target.value)} />
              </div>
              <div className="add-target">
                {activeSub
                  ? <>Клики ➜ подписка <b>«{activeSub.name}»</b></>
                  : <>Клики ➜ создадут подписку</>}
              </div>
            </div>
          )}
          <div className="col-body">
            {panels.length === 0 ? (
              <div className="empty-state">
                <div className="icon">📭</div>
                <p>Нет панелей</p>
                <p className="hint">Добавьте первую 3X-UI панель, чтобы начать собирать подписки</p>
                <button className="btn btn-primary" onClick={() => setShowAddPanel(true)}>+ Добавить панель</button>
              </div>
            ) : (
              <>
                {loadingClients ? (
                  <div className="loading-state"><div className="spin"></div><p>Загружаем клиенты с {panel?.name}…</p></div>
                ) : clientsError ? (
                  <div className="empty-state">
                    <div className="icon">⚠️</div>
                    <p>Панель недоступна</p>
                    <p className="hint">{clientsError}</p>
                  </div>
                ) : filteredClients.length === 0 ? (
                  <div className="empty-state">
                    <div className="icon">👤</div>
                    <p>{clients.length === 0 ? 'На панели нет клиентов' : 'Клиенты не найдены'}</p>
                    <p className="hint">{clients.length === 0 ? 'Добавьте клиента или выберите другую панель' : 'Попробуйте изменить запрос'}</p>
                    {clients.length === 0 && (
                      <button className="btn" onClick={() => setShowAddClient(true)}>+ Новый клиент</button>
                    )}
                  </div>
                ) : (
                  filteredClients.map(c => (
                    <ClientCard
                      key={c.id}
                      client={c}
                      inbounds={inbounds}
                      keySources={keySources}
                      activeSubKeys={activeSubKeys.current}
                      panelName={panel?.name || ''}
                      onChipClick={handleChipClick}
                      onOpenClient={setEditingClient}
                      panelId={panel?.id || ''}
                    />
                  ))
                )}
              </>
            )}
          </div>
        </section>

        {/* ─── Right: subscriptions ─── */}
        <section className="col">
          <div className="col-header">
            <h2>📡 Подписки</h2>
            <div className="right">
              <span className="badge">{subscriptions.length}</span>
              <button className="btn btn-sm btn-primary" onClick={() => { setPendingKS(null); setShowNewSub(true); }}>+ Новая подписка</button>
            </div>
          </div>

          {sortedSubs.length > 0 && (
            <div className="sub-tabs-area">
              <select className="sub-select" value={activeSubId || ''} onChange={e => setActiveSubId(e.target.value)}>
                {sortedSubs.map(s => (
                  <option key={s.id} value={s.id}>
                    {s.name}{s.status === 'active' && s.synced === false ? ' ⚠' : ''} · {(s.keys || []).length}
                  </option>
                ))}
              </select>
            </div>
          )}

          <div className="sub-detail">
            {!activeSub ? (
              <div className="empty-state" style={{ marginTop: 40 }}>
                <div className="icon">📡</div>
                <p>Нет подписок</p>
                <p className="hint">Создайте первую — затем добавляйте чипсы KeySource слева</p>
                <button className="btn btn-primary" onClick={() => { setPendingKS(null); setShowNewSub(true); }}>+ Новая подписка</button>
              </div>
            ) : (
              <SubscriptionDetail
                sub={activeSub}
                keySourceById={keySourceById}
                testingKs={testingKs}
                generating={generating}
                testingSub={testingSubs.has(activeSub.id)}
                testResults={subTestResults[activeSub.id] || null}
                testableCount={testableCount}
                onGenerate={() => handleGenerate(activeSub)}
                onTestSub={() => handleTestSub(activeSub)}
                onDelete={() => setDeleteSub(activeSub)}
                onAddManual={() => setShowAddManualKS(true)}
                onCopyValue={async (text, label) => {
                  const ok = await copyToClipboard(text);
                  showToast(ok ? `📋 ${label} скопировано: ${text}` : '⚠️ Не удалось скопировать', ok ? 'ok' : 'err');
                }}
                onCopyKey={copyKSKey}
                onRemoveKey={(subKey) => handleRemoveKey(activeSub.id, subKey)}
                onTestKS={handleTestKS}
                onOpenKS={(ks, subKey) => setKsDetails({ keySource: ks, subKey })}
                onRefresh={loadData}
                onCopyKeyFromChip={(ksOrLink) => {
                  if (typeof ksOrLink === 'string') {
                    copyToClipboard(ksOrLink).then(ok => showToast(ok ? '🔑 Ключ скопирован' : '⚠️ Не удалось скопировать', ok ? 'ok' : 'err'));
                  } else {
                    copyKSKey(ksOrLink);
                  }
                }}
              />
            )}
          </div>
        </section>
        </div>
        )}

        {/* ─── Modals ─── */}
      {showAddPanel && <AddPanelModal onClose={() => setShowAddPanel(false)} onSubmit={handleAddPanel} />}
      {showAddClient && <AddClientModal inbounds={inbounds} onClose={() => setShowAddClient(false)} onSubmit={handleCreateClient} />}
      {showAddManualKS && (
        <AddManualKSModal
          onClose={() => setShowAddManualKS(false)}
          onSubmit={async (data) => { setShowAddManualKS(false); await handleAddManualKS(data); }}
        />
      )}
      {editingClient && (
        <EditClientModal
          client={editingClient}
          allInbounds={inbounds}
          onClose={() => setEditingClient(null)}
          onAttachInbound={handleAttachInbound}
          onDetachInbound={handleDetachInbound}
          onSave={handleUpdateClient}
        />
      )}

      {showNewSub && (
        <NewSubModal
          onClose={() => { setShowNewSub(false); setPendingKS(null); }}
          onSubmit={handleNewSub}
          existingNames={subscriptions.map(s => s.name)}
          hint={pendingKS ? 'Чипс добавлен в очередь — он попадёт в подписку сразу после создания.' : null}
        />
      )}

      {ksDetails && (() => {
        const { keySource, subKey } = ksDetails;
        const usedInSubs = keySource ? subscriptions.filter(s => (s.keys || []).some(k => k.keySourceId === keySource.id)).length : 0;
        const inThisSub = !!activeSub && (subKey ? true : (activeSub.keys || []).some(k => k.keySourceId === keySource.id));
        return (
          <KSDetailsModal
            keySource={keySource}
            usedInSubs={usedInSubs}
            inThisSub={inThisSub}
            subKey={subKey}
            onClose={() => setKsDetails(null)}
            onCopyKey={copyKSKey}
            onDelete={() => setDeleteKS(keySource)}
            onTest={() => handleTestKS(keySource)}
            testing={keySource ? testingKs === keySource.id : false}
          />
        );
      })()}

      {deleteSub && (
        <DeleteSubModal
          sub={deleteSub}
          onClose={() => setDeleteSub(null)}
          onConfirm={() => handleDeleteSub(deleteSub)}
        />
      )}

      {deleteKS && (() => {
        const usedInSubs = subscriptions.filter(s => (s.keys || []).some(k => k.keySourceId === deleteKS.id)).length;
        const subNames = subscriptions.filter(s => (s.keys || []).some(k => k.keySourceId === deleteKS.id)).map(s => s.name);
        return (
          <DeleteKSModal
            keySource={deleteKS}
            usedInSubs={usedInSubs}
            subNames={subNames}
            onClose={() => setDeleteKS(null)}
            onConfirm={() => handleDeleteKS(deleteKS)}
          />
        );
      })()}

      {report && (
        <ReportModal
          subName={report.subName}
          report={report.report}
          included={report.included}
          skipped={report.skipped}
          onClose={() => setReport(null)}
        />
      )}
      </div>
    </div>
  );
}

export default function App() {
  return (
    <ToastProvider>
      <AuthGate>
        <AppInner />
      </AuthGate>
    </ToastProvider>
  );
}
