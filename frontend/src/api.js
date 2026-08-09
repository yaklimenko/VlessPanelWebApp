const API_BASE = '/api';

async function request(url, options = {}) {
  const res = await fetch(`${API_BASE}${url}`, {
    headers: { 'Content-Type': 'application/json', ...options.headers },
    ...options,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || `HTTP ${res.status}`);
  }
  return res.json();
}

export const api = {
  // Panels
  listPanels: () => request('/panels'),
  createPanel: (data) => request('/panels', { method: 'POST', body: JSON.stringify(data) }),
  deletePanel: (id) => request(`/panels/${id}`, { method: 'DELETE' }),
  listClients: (panelId) => request(`/panels/${panelId}/clients`),
  createClient: (panelId, data) => request(`/panels/${panelId}/clients`, { method: 'POST', body: JSON.stringify(data) }),
  getClientKeys: (panelId, email) => request(`/panels/${panelId}/clients/${encodeURIComponent(email)}/keys`),
  attachInbound: (panelId, email, data) => request(`/panels/${panelId}/clients/${encodeURIComponent(email)}/attach`, { method: 'POST', body: JSON.stringify(data) }),
  detachInbound: (panelId, email, data) => request(`/panels/${panelId}/clients/${encodeURIComponent(email)}/detach`, { method: 'POST', body: JSON.stringify(data) }),
  updateClient: (panelId, email, data) => request(`/panels/${panelId}/clients/${encodeURIComponent(email)}/update`, { method: 'POST', body: JSON.stringify(data) }),
  listInbounds: (panelId) => request(`/panels/${panelId}/inbounds`, { method: 'POST' }),

  // Subscriptions
  listSubscriptions: () => request('/subscriptions'),
  createSubscription: (data) => request('/subscriptions', { method: 'POST', body: JSON.stringify(data) }),
  getSubscription: (id) => request(`/subscriptions/${id}`),
  updateSubscription: (id, data) => request(`/subscriptions/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteSubscription: (id) => request(`/subscriptions/${id}`, { method: 'DELETE' }),
  getSubscriptionRaw: async (id) => {
    const res = await fetch(`${API_BASE}/subscriptions/${id}/raw`);
    if (!res.ok) throw new Error('Failed to get raw');
    return res.text();
  },
  testSubscription: (id) => request(`/subscriptions/${id}/test`, { method: 'POST' }),

  // Key sources
  listKeySources: () => request('/key-sources'),
  createKeySource: (data) => request('/key-sources', { method: 'POST', body: JSON.stringify(data) }),
  deleteKeySource: (id) => request(`/key-sources/${id}`, { method: 'DELETE' }),
  getKeySourceKey: (id) => request(`/key-sources/${id}/key`),
  testKeySource: (id) => request(`/key-sources/${id}/test`),
  getKeySourceTraffic: (id) => request(`/key-sources/${id}/traffic`),

  // Sync with aggregator
  syncToAggregator: () => request('/sync', { method: 'POST' }),
};
