import React, { useState, useEffect, useCallback, useRef } from 'react';
import { api } from './api';
import {
  ToastProvider, useToast,
  Header, ClientCard, KSChip,
  NewSubModal, KSDetailsModal, DeleteSubModal, DeleteKSModal, ReportModal,
  AddPanelModal, AddClientModal, EditClientModal,
  fmtDate, fmtShortDate, fmtDateTime,
} from './components';

const panelHost = (p) => { try { return new URL(p.url).hostname; } catch { return p.url || ''; } };

function AppInner() {
  const showToast = useToast();

  // Panels / clients
  const [panels, setPanels] = useState([]);
  const [currentPanelId, setCurrentPanelId] = useState(null);
  const [clients, setClients] = useState([]);
  const [inbounds, setInbounds] = useState([]);
  const [clientsError, setClientsError] = useState(null);
  const [clientSearch, setClientSearch] = useState('');
  const [loadingClients, setLoadingClients] = useState(false);
  const [showAddPanel, setShowAddPanel] = useState(false);
  const [showAddClient, setShowAddClient] = useState(false);
  const [editingClient, setEditingClient] = useState(null);

  // KeySources
  const [keySources, setKeySources] = useState([]);
  const [testingKs, setTestingKs] = useState(null); // ks id being tested

  // Subscriptions
  const [subscriptions, setSubscriptions] = useState([]);
  const [activeSubId, setActiveSubId] = useState(null);
  const [generating, setGenerating] = useState(false);
  const [testingSub, setTestingSub] = useState(false);
  const [subTestResults, setSubTestResults] = useState({});
  const [syncing, setSyncing] = useState(false);

  // Modals
  const [showNewSub, setShowNewSub] = useState(false);
  const [pendingKS, setPendingKS] = useState(null); // ksId to add right after sub creation
  const [ksDetails, setKsDetails] = useState(null); // {keySource, subKey}
  const [deleteSub, setDeleteSub] = useState(null);
  const [deleteKS, setDeleteKS] = useState(null);
  const [report, setReport] = useState(null); // {subName, report, included, skipped}

  const activeSub = subscriptions.find(s => s.id === activeSubId) || null;
  const activeSubKeys = useRef(new Set());
  activeSubKeys.current = new Set((activeSub?.keys || []).map(k => k.keySourceId).filter(Boolean));

  // ─── Load panels ───
  useEffect(() => {
    api.listPanels()
      .then(data => {
        setPanels(data);
        if (data.length > 0) setCurrentPanelId(prev => prev || data[0].id);
      })
      .catch(err => showToast('⚠️ ' + err.message));
  }, []);

  // ─── Sync currentPanelId with panels ───
  useEffect(() => {
    if (panels.length === 0) setCurrentPanelId(null);
    else if (currentPanelId && !panels.some(p => p.id === currentPanelId)) setCurrentPanelId(panels[0].id);
  }, [panels, currentPanelId]);

  // ─── Load clients + inbounds for panel ───
  useEffect(() => {
    if (!currentPanelId) return;
    setClients([]);
    setInbounds([]);
    setClientsError(null);
    setLoadingClients(true);
    Promise.all([api.listClients(currentPanelId), api.listInbounds(currentPanelId)])
      .then(([cl, ib]) => { setClients(cl || []); setInbounds(ib || []); })
      .catch(err => { setClientsError(err.message || 'Панель недоступна'); showToast('⚠️ ' + err.message); })
      .finally(() => setLoadingClients(false));
  }, [currentPanelId]);

  // ─── Load key sources + subscriptions ───
  const loadData = useCallback(() => {
    api.listKeySources()
      .then(setKeySources)
      .catch(err => showToast('⚠️ KeySource: ' + err.message));
    api.listSubscriptions()
      .then(data => {
        setSubscriptions(data || []);
        setActiveSubId(prev => {
          if (prev && (data || []).some(s => s.id === prev)) return prev;
          return (data && data.length > 0) ? data[0].id : null;
        });
      })
      .catch(err => showToast('⚠️ Подписки: ' + err.message));
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  // ─── Panel handlers ───
  const handleAddPanel = (data) => {
    api.createPanel(data)
      .then(panel => {
        setPanels(prev => [...prev, panel]);
        setCurrentPanelId(panel.id);
        setShowAddPanel(false);
        showToast('✅ Панель добавлена');
      })
      .catch(err => showToast('⚠️ ' + err.message));
  };

  const handleDeletePanel = () => {
    const panelId = currentPanelId;
    if (!panelId) return;
    api.deletePanel(panelId)
      .then(() => {
        const remaining = panels.filter(p => p.id !== panelId);
        setPanels(remaining);
        if (currentPanelId === panelId && remaining.length > 0) setCurrentPanelId(remaining[0].id);
        showToast('🗑 Панель удалена');
        loadData();
      })
      .catch(err => showToast('⚠️ ' + err.message));
  };

  const handleCreateClient = (data) => {
    if (!currentPanelId) return;
    api.createClient(currentPanelId, data)
      .then(() => {
        showToast('✅ Клиент создан');
        setShowAddClient(false);
        api.listClients(currentPanelId).then(d => setClients(d)).catch(() => {});
      })
      .catch(err => showToast('⚠️ ' + err.message));
  };

  const handleAttachInbound = async (email, inboundId) => {
    if (!currentPanelId) return;
    try {
      await api.attachInbound(currentPanelId, email, { inboundId });
      showToast('✅ Инбаунд добавлен');
      api.listClients(currentPanelId).then(d => setClients(d)).catch(() => {});
    } catch (err) { showToast('⚠️ ' + err.message); }
  };

  const handleDetachInbound = async (email, inboundId) => {
    if (!currentPanelId) return;
    try {
      await api.detachInbound(currentPanelId, email, { inboundId });
      showToast('🗑 Инбаунд удалён');
      api.listClients(currentPanelId).then(d => setClients(d)).catch(() => {});
    } catch (err) { showToast('⚠️ ' + err.message); }
  };

  const handleUpdateClient = async (email, expiryDate) => {
    if (!currentPanelId) return;
    try {
      await api.updateClient(currentPanelId, email, { expiryDate });
      showToast('💾 Клиент обновлён');
      setEditingClient(null);
      api.listClients(currentPanelId).then(d => setClients(d)).catch(() => {});
    } catch (err) { showToast('⚠️ ' + err.message); }
  };

  // ─── Clipboard ───
  const copyToClipboard = async (text) => {
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(text);
        return true;
      }
    } catch (_) { /* fall through */ }
    try {
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.left = '-9999px';
      document.body.appendChild(ta);
      ta.focus(); ta.select();
      const ok = document.execCommand('copy');
      document.body.removeChild(ta);
      return ok;
    } catch (_) { return false; }
  };

  // ─── KeySource resolution on chip click ───
  const handleChipClick = async (client, inbound) => {
    // 1. ensure KeySource exists (POST dedups)
    let ksId;
    try {
      const res = await api.createKeySource({
        type: 'panel',
        panelId: currentPanelId,
        clientEmail: client.email,
        inboundId: inbound.id,
      });
      ksId = res.keySource.id;
      if (!res.deduped) showToast(`🔑 KeySource создан: ${inbound.remark} · ${client.email}`);
    } catch (err) {
      showToast('⚠️ ' + err.message);
      return;
    }

    // refresh key sources list to get statuses
    api.listKeySources().then(setKeySources).catch(() => {});

    if (!activeSub) {
      // no active subscription → propose creating one
      setPendingKS(ksId);
      setShowNewSub(true);
      showToast('⚠️ Сначала создайте подписку', 'warn');
      return;
    }
    if (activeSubKeys.current.has(ksId)) {
      showToast('⚠️ Уже добавлено в «' + activeSub.name + '»', 'warn');
      return;
    }
    await addKeySourceToSub(activeSub.id, ksId);
  };

  const addKeySourceToSub = async (subId, ksId) => {
    try {
      await api.updateSubscription(subId, { addKeySourceIds: [ksId] });
      const subs = await api.listSubscriptions();
      setSubscriptions(subs || []);
      showToast('✅ Добавлено → «' + (subs.find(s => s.id === subId)?.name || '') + '»');
    } catch (err) {
      showToast('⚠️ ' + err.message);
    }
  };

  // ─── New subscription modal ───
  const handleNewSub = (name) => {
    api.createSubscription({ name, keySourceIds: [] })
      .then(res => {
        const sub = res.subscription;
        setShowNewSub(false);
        setSubscriptions(prev => [...prev, sub]);
        setActiveSubId(sub.id);
        showToast(`✅ Черновик «${name}» создан`);
        // add the pending chip if any
        if (pendingKS) {
          const id = pendingKS;
          setPendingKS(null);
          addKeySourceToSub(sub.id, id);
        }
      })
      .catch(err => showToast('⚠️ ' + err.message));
  };

  // ─── Generate / regenerate subscription ───
  const handleGenerate = (sub) => {
    if (!sub || sub.keys.length === 0 || generating) return;
    setGenerating(true);
    api.updateSubscription(sub.id, { regenerate: true })
      .then(res => {
        setSubscriptions(prev => prev.map(s => s.id === sub.id ? res.subscription : s));
        setSubTestResults(prev => ({ ...prev, [sub.id]: null }));
        setReport({
          subName: res.subscription.name,
          report: res.report || [],
          included: res.included,
          skipped: res.skipped,
        });
        const skipN = res.skipped || 0;
        showToast(skipN === 0
          ? `✅ Подписка обновлена: configs-${res.subscription.name}.txt`
          : `⚠️ Готово с пропусками: ${res.included} включено, ${skipN} пропущено`, skipN === 0 ? 'ok' : 'warn');
      })
      .catch(err => showToast('⚠️ ' + err.message))
      .finally(() => setGenerating(false));
  };

  // ─── Remove key from subscription ───
  const handleRemoveKey = async (subId, subKey) => {
    try {
      const res = await api.updateSubscription(subId, { removeKeyId: subKey.id });
      setSubscriptions(prev => prev.map(s => s.id === subId ? res : s));
      showToast('🗑 Ключ удалён из подписки');
    } catch (err) { showToast('⚠️ ' + err.message); }
  };

  // ─── Copy key ───
  const copyKSKey = async (ks) => {
    let link = (ks && ks.cachedKey && ks.cachedKey.link) || (ks && ks.vlessLink);
    try {
      if (!link && ks) {
        const res = await api.getKeySourceKey(ks.id);
        link = res.key && res.key.link;
      }
      if (!link) { showToast('⚠️ Ключ недоступен (не извлечён)'); return; }
      const ok = await copyToClipboard(link);
      showToast(ok ? '🔑 VLESS-ключ скопирован' : '⚠️ Не удалось скопировать в буфер', ok ? 'ok' : 'err');
    } catch (err) { showToast('⚠️ ' + err.message); }
  };

  const copySubLink = async (sub) => {
    const ok = await copyToClipboard('https://example.com/sub/' + sub.name);
    showToast(ok ? `📋 Ссылка скопирована: /sub/${sub.name}` : '⚠️ Не удалось скопировать', ok ? 'ok' : 'err');
  };

  // ─── Per-keySource test ───
  const handleTestKS = async (ks) => {
    if (testingKs) return;
    setTestingKs(ks.id);
    try {
      const res = await api.testKeySource(ks.id);
      // update lastTest in state
      setKeySources(prev => prev.map(k => k.id === ks.id ? { ...k, lastTest: res.lastTest } : k));
      const ok = res.lastTest && res.lastTest.status === 'ok';
      showToast(ok ? `✅ Тест: ${res.lastTest.ms || '?'} мс` : `❌ ${(res.lastTest && res.lastTest.error) || 'тест не пройден'}`, ok ? 'ok' : 'err');
    } catch (err) { showToast('⚠️ ' + err.message); }
    finally { setTestingKs(null); }
  };

  // ─── Subscription test ───
  const handleTestSub = (sub) => {
    if (!sub || testingSub) return;
    setTestingSub(true);
    api.testSubscription(sub.id)
      .then(data => {
        setSubTestResults(prev => ({ ...prev, [sub.id]: data }));
        showToast(`🧪 Тест завершён: ${data.ok}/${data.total}`, data.ok === data.total ? 'ok' : 'warn');
      })
      .catch(err => showToast('⚠️ ' + err.message))
      .finally(() => setTestingSub(false));
  };

  // ─── Delete subscription ───
  const handleDeleteSub = async (sub) => {
    try {
      await api.deleteSubscription(sub.id);
      setDeleteSub(null);
      setSubscriptions(prev => {
        const next = prev.filter(s => s.id !== sub.id);
        if (activeSubId === sub.id && next.length > 0) setActiveSubId(next[0].id);
        if (next.length === 0) setActiveSubId(null);
        return next;
      });
      showToast(`🗑 Подписка «${sub.name}» удалена`);
    } catch (err) { showToast('⚠️ ' + err.message); }
  };

  // ─── Delete KeySource ───
  const handleDeleteKS = async (ks) => {
    try {
      const res = await api.deleteKeySource(ks.id);
      setDeleteKS(null);
      setKsDetails(null);
      loadData();
      const n = res.usedInSubscriptions || 0;
      showToast(`🗑 KeySource удалён${n > 0 ? ` из ${n} ${n === 1 ? 'подписки' : 'подписок'}` : ''}`);
    } catch (err) { showToast('⚠️ ' + err.message); }
  };

  // ─── Sync with aggregator ───
  const handleSyncAll = () => {
    if (syncing) return;
    setSyncing(true);
    api.syncToAggregator()
      .then(() => {
        showToast('✅ Синхронизировано с агрегатором');
        loadData();
      })
      .catch(err => showToast('⚠️ ' + (err.message || 'синк недоступен (скрипт только на проде)')))
      .finally(() => setSyncing(false));
  };

  // ─── Derived ───
  const panel = panels.find(p => p.id === currentPanelId) || null;
  const q = clientSearch.toLowerCase();
  const filteredClients = (clients || []).filter(c =>
    !q || c.email.toLowerCase().includes(q) ||
    (c.inbounds || []).some(i => i.toLowerCase().includes(q)));

  const sortedPanels = [...panels].sort((a, b) => a.name.localeCompare(b.name, 'ru'));
  const ksCountByPanel = {};
  (keySources || []).forEach(ks => { if (ks.type === 'panel') ksCountByPanel[ks.panelId] = (ksCountByPanel[ks.panelId] || 0) + 1; });

  const keySourceById = {};
  (keySources || []).forEach(ks => { keySourceById[ks.id] = ks; });

  const sortedSubs = [...subscriptions].sort((a, b) => a.name.localeCompare(b.name, 'ru'));

  const testableCount = (activeSub?.keys || []).filter(k => k.link).length;

  return (
    <div className="app">
      <Header
        panels={panels}
        selectedPanelId={currentPanelId}
        onPanelChange={setCurrentPanelId}
        onAddPanel={() => setShowAddPanel(true)}
        onDeletePanel={handleDeletePanel}
        onSyncAll={handleSyncAll}
        syncing={syncing}
      />

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
                    {s.name}{s.status === 'active' && s.synced === false ? ' ⚠' : ''} · {s.keys.length}
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
                testingSub={testingSub}
                testResults={subTestResults[activeSub.id] || null}
                testableCount={testableCount}
                onGenerate={() => handleGenerate(activeSub)}
                onTestSub={() => handleTestSub(activeSub)}
                onCopyLink={() => copySubLink(activeSub)}
                onDelete={() => setDeleteSub(activeSub)}
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

      {/* ─── Modals ─── */}
      {showAddPanel && <AddPanelModal onClose={() => setShowAddPanel(false)} onSubmit={handleAddPanel} />}
      {showAddClient && <AddClientModal inbounds={inbounds} onClose={() => setShowAddClient(false)} onSubmit={handleCreateClient} />}
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
        const subNames = keySource ? subscriptions.filter(s => (s.keys || []).some(k => k.keySourceId === keySource.id)).map(s => s.name) : [];
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
  );
}

// ─── Subscription detail panel ───
function SubscriptionDetail({
  sub, keySourceById, testingKs, generating, testingSub, testResults, testableCount,
  onGenerate, onTestSub, onCopyLink, onDelete,
  onCopyKey, onCopyKeyFromChip, onRemoveKey, onTestKS, onOpenKS, onRefresh,
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
          🔑 {hasKeys ? sub.keys.length : 0} {sub.keys.length === 1 ? 'ключ' : (sub.keys.length < 5 ? 'ключа' : 'ключей')} · порядок = порядок добавления
        </div>
        <div className="sub-actions">
          <button className="btn btn-success" onClick={onGenerate} disabled={!hasKeys || generating}>
            {generating ? <span className="spin"></span> : genLabel}
          </button>
          <button className="btn" onClick={onTestSub} disabled={!testableCount || testingSub}>
            {testingSub ? <span className="spin"></span> : '🧪 Тест подписки'}
          </button>
          <button className="btn btn-sm" onClick={onCopyLink} disabled={sub.status !== 'active'} title="Скопировать ссылку подписки">⧉ ссылка</button>
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
                <span>Файл:</span><code>configs-{sub.name}.txt</code>
                <span>·</span><span>Ссылка:</span><code>https://example.com/sub/{sub.name}</code>
              </div>
              <div className="row">
                <span>Локально (mtime):</span><code>{fmtDateTime(sub.fileMtime) || '—'}</code>
                <span>·</span><span>Агрегатор (проверка):</span><code>{fmtDateTime(sub.aggrLastModified) || '—'}</code>
                {sub.synced === true && <span className="sync-ok">— версии совпадают</span>}
                {sub.synced === false && <span className="sync-warn">— отличается, нужен rsync</span>}
                {sub.synced == null && <span className="sync-warn">— агрегатор недоступен</span>}
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

export default function App() {
  return (
    <ToastProvider>
      <AppInner />
    </ToastProvider>
  );
}
