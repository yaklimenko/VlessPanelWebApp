import React, { useState, useEffect } from 'react';
import { getToken, setToken, loadConfig } from '../api';

function Login({ onLogin }) {
  const [value, setValue] = useState('');
  const [error, setError] = useState('');
  const submit = (e) => {
    e.preventDefault();
    const t = value.trim();
    if (!t) { setError('Введите токен'); return; }
    setToken(t);
    onLogin(t);
  };
  return (
    <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', background: '#0f1115', color: '#e6e6e6' }}>
      <form onSubmit={submit} style={{ background: '#1a1d24', padding: 32, borderRadius: 12, width: 360, boxShadow: '0 8px 30px rgba(0,0,0,.4)' }}>
        <h2 style={{ margin: '0 0 8px' }}>VlessPanel</h2>
        <p style={{ margin: '0 0 20px', color: '#9aa0aa', fontSize: 14 }}>Введите API-токен для входа</p>
        <input
          type="password"
          autoFocus
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="vlt_…"
          style={{ width: '100%', boxSizing: 'border-box', padding: '10px 12px', borderRadius: 8, border: '1px solid #2a2f3a', background: '#0f1115', color: '#e6e6e6', marginBottom: 12 }}
        />
        {error && <div style={{ color: '#f66', marginBottom: 12, fontSize: 13 }}>{error}</div>}
        <button type="submit" style={{ width: '100%', padding: '10px 12px', borderRadius: 8, border: 'none', background: '#3b82f6', color: '#fff', fontWeight: 600, cursor: 'pointer' }}>Войти</button>
      </form>
    </div>
  );
}

export function AuthGate({ children }) {
  const [token, setTokenState] = useState(getToken());
  const [authEnabled, setAuthEnabled] = useState(null);

  useEffect(() => {
    loadConfig();
    fetch('/api/auth-status')
      .then((r) => r.json())
      .then((d) => setAuthEnabled(!!d.enabled))
      .catch(() => setAuthEnabled(false));
  }, []);

  useEffect(() => {
    const onUnauthorized = () => {
      setToken('');
      setTokenState('');
    };
    window.addEventListener('vlesspanel:unauthorized', onUnauthorized);
    return () => window.removeEventListener('vlesspanel:unauthorized', onUnauthorized);
  }, []);

  if (authEnabled === null) return null;
  if (!authEnabled) return children;
  if (!token) return <Login onLogin={setTokenState} />;
  return children;
}
