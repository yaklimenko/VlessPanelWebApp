import React, { useState, useEffect, useCallback } from 'react';
import { api } from './api';
import {
  ToastProvider, useToast,
  Header, ClientCard, SubscriptionCard,
  AddPanelModal, AddClientModal, AddSubscriptionModal,
} from './components';

function AppInner() {
  const showToast = useToast();

  // Panels
  const [panels, setPanels] = useState([]);
  const [currentPanelId, setCurrentPanelId] = useState(null);
  const [showAddPanel, setShowAddPanel] = useState(false);

  // Clients
  const [clients, setClients] = useState([]);
  const [selectedClientId, setSelectedClientId] = useState(null);
  const [clientSearch, setClientSearch] = useState('');
  const [showAddClient, setShowAddClient] = useState(false);
  const [inbounds, setInbounds] = useState([]);
  const [loadingClients, setLoadingClients] = useState(false);

  // Subscriptions
  const [subscriptions, setSubscriptions] = useState([]);
  const [openSubId, setOpenSubId] = useState(null);
  const [showAddSub, setShowAddSub] = useState(false);
  const [newKeyInputs, setNewKeyInputs] = useState({});
  const [showAddKeyForms, setShowAddKeyForms] = useState({});
  const [testingSubs, setTestingSubs] = useState({});
  const [testResults, setTestResults] = useState({});

  // ─── Load panels ───
  useEffect(() => {
    api.listPanels()
      .then(data => {
        setPanels(data);
        if (data.length > 0 && !currentPanelId) {
          setCurrentPanelId(data[0].id);
        }
      })
      .catch(err => showToast('⚠️ ' + err.message));
  }, []);

  // ─── Sync currentPanelId with panels array ───
  useEffect(() => {
    if (panels.length === 0) {
      setCurrentPanelId(null);
    } else if (currentPanelId && !panels.some(p => p.id === currentPanelId)) {
      setCurrentPanelId(panels[0].id);
    }
  }, [panels, currentPanelId]);

  // ─── Load clients ───
  useEffect(() => {
    if (!currentPanelId) return;
    setLoadingClients(true);
    Promise.all([
      api.listClients(currentPanelId),
      api.listInbounds(currentPanelId),
    ])
      .then(([clientsData, inboundsData]) => {
        setClients(clientsData || []);
        setInbounds(inboundsData || []);
      })
      .catch(err => showToast('⚠️ ' + err.message))
      .finally(() => setLoadingClients(false));
  }, [currentPanelId]);

  // ─── Load subscriptions ───
  const loadSubscriptions = useCallback(() => {
    api.listSubscriptions()
      .then(data => setSubscriptions(data || []))
      .catch(err => showToast('⚠️ ' + err.message));
  }, []);

  useEffect(() => { loadSubscriptions(); }, [loadSubscriptions]);

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
    if (panels.length <= 1) {
      showToast('⚠️ Нельзя удалить последнюю панель');
      return;
    }
    const panelId = currentPanelId;
    if (!panelId) return;
    api.deletePanel(panelId)
      .then(() => {
        const remaining = panels.filter(p => p.id !== panelId);
        setPanels(remaining);
        if (currentPanelId === panelId && remaining.length > 0) {
          setCurrentPanelId(remaining[0].id);
        }
        showToast('🗑 Панель удалена');
      })
      .catch(err => showToast('⚠️ ' + err.message));
  };

  // ─── Client handlers ───
  const handleCreateClient = (data) => {
    if (!currentPanelId) return;
    api.createClient(currentPanelId, data)
      .then(() => {
        showToast('✅ Клиент создан');
        setShowAddClient(false);
        // Reload clients
        api.listClients(currentPanelId).then(d => setClients(d)).catch(() => {});
      })
      .catch(err => showToast('⚠️ ' + err.message));
  };

  const handleCopyKey = (key) => {
    navigator.clipboard.writeText(key.link).catch(() => {});
    showToast(`🔑 Ключ «${key.label}» скопирован`);
  };

  // ─── Subscription handlers ───
  const handleCreateSub = (data) => {
    api.createSubscription(data)
      .then(() => {
        loadSubscriptions();
        setShowAddSub(false);
        showToast('✅ Подписка создана');
      })
      .catch(err => showToast('⚠️ ' + err.message));
  };

  const handleDeleteSub = (subId) => {
    api.deleteSubscription(subId)
      .then(() => {
        loadSubscriptions();
        showToast('🗑 Подписка удалена');
      })
      .catch(err => showToast('⚠️ ' + err.message));
  };

  const handleCopySubLink = (sub) => {
    navigator.clipboard.writeText(sub.link || `config-${sub.name}.txt`).catch(() => {});
    showToast(`📋 Ссылка подписки скопирована`);
  };

  const handleRefreshSub = () => {
    loadSubscriptions();
    showToast('🔄 Подписки обновлены');
  };

  const handleCopyVlessKey = (key) => {
    navigator.clipboard.writeText(key.link).catch(() => {});
    showToast(`🔑 VLESS-ключ скопирован`);
  };

  const handleDeleteKey = (subId, keyId) => {
    const sub = subscriptions.find(s => s.id === subId);
    if (!sub) return;
    const newKeys = (sub.keys || []).filter(k => k.id !== keyId);
    api.updateSubscription(subId, { name: sub.name, keys: newKeys })
      .then(() => {
        loadSubscriptions();
        showToast('🗑 Ключ удалён');
      })
      .catch(err => showToast('⚠️ ' + err.message));
  };

  const handleToggleAddKeyForm = (subId) => {
    setShowAddKeyForms(prev => ({ ...prev, [subId]: true }));
    setNewKeyInputs(prev => ({ ...prev, [subId]: '' }));
  };

  const handleNewKeyChange = (subId, value) => {
    setNewKeyInputs(prev => ({ ...prev, [subId]: value }));
  };

  const handleNewKeyConfirm = (subId) => {
    const val = (newKeyInputs[subId] || '').trim();
    if (!val.startsWith('vless://') && !val.startsWith('vmess://')) {
      showToast('⚠️ Ссылка должна начинаться с vless:// или vmess://');
      return;
    }
    const sub = subscriptions.find(s => s.id === subId);
    if (!sub) return;
    const newKeys = [...(sub.keys || []), { id: 'k-' + Date.now(), link: val }];
    api.updateSubscription(subId, { name: sub.name, keys: newKeys })
      .then(() => {
        loadSubscriptions();
        setShowAddKeyForms(prev => ({ ...prev, [subId]: false }));
        setNewKeyInputs(prev => ({ ...prev, [subId]: '' }));
        showToast('✅ Ключ добавлен');
      })
      .catch(err => showToast('⚠️ ' + err.message));
  };

  const handleTest = (subId) => {
    setTestingSubs(prev => ({ ...prev, [subId]: true }));
    api.testSubscription(subId)
      .then(data => {
        setTestResults(prev => ({ ...prev, [subId]: data.result }));
        showToast('🧪 Тест завершён');
      })
      .catch(err => showToast('⚠️ ' + err.message))
      .finally(() => setTestingSubs(prev => ({ ...prev, [subId]: false })));
  };

  // ─── Filtered clients ───
  const filteredClients = (clients || []).filter(c => {
    if (!clientSearch) return true;
    const q = clientSearch.toLowerCase();
    return c.email.toLowerCase().includes(q) ||
           (c.inbounds || []).some(i => i.toLowerCase().includes(q));
  });

  return (
    <div className="app">
      <Header
        panels={panels}
        selectedPanelId={currentPanelId}
        onPanelChange={setCurrentPanelId}
        onAddPanel={() => setShowAddPanel(true)}
        onDeletePanel={handleDeletePanel}
      />

      <div className="main">
        {/* Left: Clients */}
        <div className="panel-column">
          <div className="column-header">
            <h2>📋 Клиенты</h2>
            <span className="badge">{filteredClients.length}</span>
          </div>
          <div className="column-body">
            <div className="search-wrap">
              <span className="search-icon">🔍</span>
              <input
                type="text"
                placeholder="Поиск клиентов..."
                value={clientSearch}
                onChange={e => setClientSearch(e.target.value)}
              />
            </div>
            {loadingClients ? (
              <div className="empty-state"><p>Загрузка...</p></div>
            ) : filteredClients.length === 0 ? (
              <div className="empty-state"><div className="icon">📭</div><p>Клиенты не найдены</p></div>
            ) : (
              filteredClients.map(c => (
                <ClientCard
                  key={c.id}
                  client={c}
                  isSelected={selectedClientId === c.id}
                  onToggle={() => setSelectedClientId(prev => prev === c.id ? null : c.id)}
                  onAddKey={(client) => {
                    setShowAddClient(false);
                    setSelectedClientId(client.id);
                    showToast(`🔑 Создание ключа для ${client.email}`);
                  }}
                  onCopyKey={handleCopyKey}
                />
              ))
            )}
          </div>
          <div className="column-footer">
            <button
              className="btn btn-primary"
              style={{ width: '100%' }}
              onClick={() => {
                if (!currentPanelId) { showToast('⚠️ Сначала выберите панель'); return; }
                setShowAddClient(true);
              }}
            >+ Новый клиент</button>
          </div>
        </div>

        {/* Right: Subscriptions */}
        <div className="panel-column">
          <div className="column-header">
            <h2>📡 Подписки</h2>
            <span className="badge">{subscriptions.length}</span>
          </div>
          <div className="column-body">
            {subscriptions.length === 0 ? (
              <div className="empty-state"><div className="icon">📡</div><p>Нет подписок</p></div>
            ) : (
              subscriptions.map(sub => (
                <SubscriptionCard
                  key={sub.id}
                  subscription={sub}
                  isOpen={openSubId === sub.id}
                  onToggle={() => setOpenSubId(prev => prev === sub.id ? null : sub.id)}
                  onCopyLink={() => handleCopySubLink(sub)}
                  onRefresh={handleRefreshSub}
                  onDelete={() => handleDeleteSub(sub.id)}
                  onCopyKey={handleCopyVlessKey}
                  onDeleteKey={(keyId) => handleDeleteKey(sub.id, keyId)}
                  onAddKey={() => handleToggleAddKeyForm(sub.id)}
                  newKeyValue={newKeyInputs[sub.id] || ''}
                  onNewKeyChange={(v) => handleNewKeyChange(sub.id, v)}
                  onNewKeyConfirm={() => handleNewKeyConfirm(sub.id)}
                  onNewKeyCancel={() => {
                    setShowAddKeyForms(prev => ({ ...prev, [sub.id]: false }));
                    setNewKeyInputs(prev => ({ ...prev, [sub.id]: '' }));
                  }}
                  onTest={() => handleTest(sub.id)}
                  showAddForm={showAddKeyForms[sub.id] || false}
                  testing={testingSubs[sub.id] || false}
                  testResults={testResults[sub.id] || ''}
                />
              ))
            )}
          </div>
          <div className="column-footer">
            <button
              className="btn btn-primary"
              style={{ width: '100%' }}
              onClick={() => setShowAddSub(true)}
            >+ Новая подписка для клиента</button>
          </div>
        </div>
      </div>

      {/* Modals */}
      {showAddPanel && (
        <AddPanelModal
          onClose={() => setShowAddPanel(false)}
          onSubmit={handleAddPanel}
        />
      )}
      {showAddClient && (
        <AddClientModal
          inbounds={inbounds}
          onClose={() => setShowAddClient(false)}
          onSubmit={handleCreateClient}
        />
      )}
      {showAddSub && (
        <AddSubscriptionModal
          onClose={() => setShowAddSub(false)}
          onSubmit={handleCreateSub}
        />
      )}
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
