import { useState, useEffect, useCallback, useRef } from 'react';
import { api } from '../api';
import { useToast } from '../components/Toast';

export function useVlessPanel() {
  const showToast = useToast();

  // Panels / clients
  const [panels, setPanels] = useState([]);
  const [currentPanelId, setCurrentPanelId] = useState(() => { try { return localStorage.getItem('vlesspanel:panelId') || null; } catch { return null; } });
  const [clients, setClients] = useState([]);
  const [inbounds, setInbounds] = useState([]);
  const [clientsError, setClientsError] = useState(null);
  const [clientSearch, setClientSearch] = useState('');
  const [loadingClients, setLoadingClients] = useState(false);
  const [showAddPanel, setShowAddPanel] = useState(false);
  const [showAddClient, setShowAddClient] = useState(false);
  const [showAddManualKS, setShowAddManualKS] = useState(false);
  const [editingClient, setEditingClient] = useState(null);

  // KeySources
  const [keySources, setKeySources] = useState([]);
  const [testingKs, setTestingKs] = useState(null); // ks id being tested

  // Subscriptions
  const [subscriptions, setSubscriptions] = useState([]);
  const [activeSubId, setActiveSubId] = useState(() => { try { return localStorage.getItem('vlesspanel:subId') || null; } catch { return null; } });
  const [generating, setGenerating] = useState(false);
  const [testingSubs, setTestingSubs] = useState(new Set());
  const [subTestResults, setSubTestResults] = useState({});
  const [syncing, setSyncing] = useState(false);

  // Modals
  const [showNewSub, setShowNewSub] = useState(false);
  const [pendingKS, setPendingKS] = useState(null); // ksId to add right after sub creation
  const [ksDetails, setKsDetails] = useState(null); // {keySource, subKey}
  const [deleteSub, setDeleteSub] = useState(null);
  const [deleteKS, setDeleteKS] = useState(null);
  const [report, setReport] = useState(null); // {subName, report, included, skipped}
  const [regenerating, setRegenerating] = useState(false);

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
    if (panels.length > 0 && currentPanelId && !panels.some(p => p.id === currentPanelId)) {
      setCurrentPanelId(panels[0].id);
    }
  }, [panels, currentPanelId]);

  // ─── Persist selected panel/subscription to localStorage ───
  useEffect(() => {
    try { if (currentPanelId) localStorage.setItem('vlesspanel:panelId', currentPanelId); else localStorage.removeItem('vlesspanel:panelId'); } catch {}
  }, [currentPanelId]);

  useEffect(() => {
    try { if (activeSubId) localStorage.setItem('vlesspanel:subId', activeSubId); else localStorage.removeItem('vlesspanel:subId'); } catch {}
  }, [activeSubId]);

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

  // ─── Rename panel (меняется только name; panelId остаётся — кейсорцы/подписки не ломаются) ───
  const handleRenamePanel = (panelId, name) => {
    if (!panelId) return;
    api.updatePanel(panelId, { name })
      .then(updated => {
        setPanels(prev => prev.map(p => (p.id === updated.id ? updated : p)));
        showToast('✏️ Панель переименована');
        loadData();
      })
      .catch(err => showToast('⚠️ ' + err.message));
  };

  const handleCreateClient = (data) => {
    if (!currentPanelId) return;
    api.createClient(currentPanelId, data)
      .then(() => {
        setShowAddClient(false);
        api.listClients(currentPanelId).then(d => setClients(d)).catch(() => {});
      })
      .catch(err => showToast('⚠️ ' + err.message));
  };

  const refreshClientsAndEditing = (d) => {
    setClients(d);
    // Модалка редактирования держит снапшот клиента — синхронизируем его со свежим списком,
    // чтобы добавленный/удалённый инбаунд сразу отобразился в модалке.
    setEditingClient(prev => (prev ? d.find(c => c.email === prev.email) || prev : prev));
  };

  const handleAttachInbound = async (email, inboundId) => {
    if (!currentPanelId) return;
    try {
      await api.attachInbound(currentPanelId, email, { inboundId });
      showToast('✅ Инбаунд добавлен');
      api.listClients(currentPanelId).then(refreshClientsAndEditing).catch(() => {});
    } catch (err) { showToast('⚠️ ' + err.message); }
  };

  const handleDetachInbound = async (email, inboundId) => {
    if (!currentPanelId) return;
    try {
      await api.detachInbound(currentPanelId, email, { inboundId });
      showToast('🗑 Инбаунд удалён');
      api.listClients(currentPanelId).then(refreshClientsAndEditing).catch(() => {});
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
      if (!res.deduped) {
        // Добавить локально, не дёргая панели (полный refresh — по кнопке «Обновить»)
        const panelName = (panels || []).find(p => p.id === currentPanelId)?.name || '';
        const expireDate = client.expiryTime ? new Date(client.expiryTime).toISOString().slice(0, 10) : undefined;
        setKeySources(prev => [...prev, {
          ...res.keySource,
          status: 'ok',
          panelName,
          inboundRemark: inbound.remark,
          inboundPort: inbound.port,
          clientEnabled: client.enable,
          expireDate,
          traffic: (client.up != null || client.down != null) ? { up: client.up || 0, down: client.down || 0 } : undefined,
          usedInSubs: 0,
        }]);
      }
    } catch (err) {
      showToast('⚠️ ' + err.message);
      return;
    }

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

  // ─── Manual KeySource (raw vless link) ───
  const handleAddManualKS = async ({ link, label }) => {
    let ksId;
    try {
      const res = await api.createKeySource({ type: 'manual', vlessLink: link, label });
      ksId = res.keySource.id;
      if (!res.deduped) {
        const ks = res.keySource;
        setKeySources(prev => [...prev, {
          ...ks,
          status: 'ok',
          keyAvailable: !!ks.vlessLink,
          cachedKey: ks.vlessLink ? { link: ks.vlessLink, fetchedAt: new Date().toISOString() } : undefined,
          usedInSubs: 0,
        }]);
      } else {
        showToast('⚠️ Такой ключ уже есть в источниках', 'warn');
      }
    } catch (err) {
      showToast('⚠️ ' + err.message);
      return;
    }

    if (!activeSub) {
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
    if (!sub || (sub.keys || []).length === 0 || generating) return;
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
    if (!sub || testingSubs.has(sub.id)) return;
    setTestingSubs(prev => new Set(prev).add(sub.id));
    api.testSubscription(sub.id)
      .then(data => {
        setSubTestResults(prev => ({ ...prev, [sub.id]: data }));
        showToast(`🧪 Тест завершён: ${data.ok}/${data.total}`, data.ok === data.total ? 'ok' : 'warn');
      })
      .catch(err => showToast('⚠️ ' + err.message))
      .finally(() => setTestingSubs(prev => { const n = new Set(prev); n.delete(sub.id); return n; }));
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
      await api.deleteKeySource(ks.id);
      setDeleteKS(null);
      setKsDetails(null);
      loadData();
    } catch (err) { showToast('⚠️ ' + err.message); }
  };

  const handleRegenerateAll = () => {
    if (regenerating) return;
    if (!window.confirm('Перегенерировать все подписки с panel-KeySource?\nСвежие ключи будут запрошены с панелей (manual-ключи сохранятся).')) return;
    setRegenerating(true);
    api.regenerateAllSubscriptions()
      .then(res => {
        const done = res.regenerated || 0;
        const skipped = res.skipped || 0;
        const failed = (res.results || []).filter(x => !x.regenerated && x.reason && x.reason !== 'нет panel KeySource').length;
        showToast(`✅ Перегенерировано: ${done}${skipped ? `, пропущено: ${skipped}` : ''}${failed ? `, ошибок: ${failed}` : ''}`);
        loadData();
      })
      .catch(err => showToast('⚠️ ' + (err.message || 'ошибка перегенерации')))
      .finally(() => setRegenerating(false));
  };

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

  return {
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
    handleAddPanel, handleDeletePanel, handleRenamePanel, handleCreateClient, handleAttachInbound,
    handleDetachInbound, handleUpdateClient, copyToClipboard, handleChipClick,
    handleAddManualKS, addKeySourceToSub, handleNewSub, handleGenerate,
    handleRemoveKey, copyKSKey, handleTestKS, handleTestSub, handleDeleteSub,
    handleDeleteKS, handleRegenerateAll, handleSyncAll,
  };
}
