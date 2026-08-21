import React from 'react';
import { Modal } from './Modal';

export function AddPanelModal({ onClose, onSubmit }) {
  const [name, setName] = React.useState('');
  const [url, setUrl] = React.useState('');
  const [token, setToken] = React.useState('');
  const [webBasePath, setWebBasePath] = React.useState('');
  const [skipVerify, setSkipVerify] = React.useState(false);

  const handleSubmit = (e) => {
    e.preventDefault();
    if (!name || !url || !token) return;
    onSubmit({ name, url: url.replace(/\/+$/, ''), token, webBasePath: webBasePath.replace(/\/+$/, ''), insecureSkipVerify: skipVerify });
  };

  return (
    <Modal title="➕ Добавить панель" onClose={onClose}>
      <form onSubmit={handleSubmit} className="modal-form">
        <div className="form-group">
          <label>Название</label>
          <input type="text" value={name} onChange={e => setName(e.target.value)} placeholder="Hip Warsaw" required autoFocus />
        </div>
        <div className="form-group">
          <label>URL</label>
          <input type="text" value={url} onChange={e => setUrl(e.target.value)} placeholder="https://203.0.113.4:2053" required />
        </div>
        <div className="form-group">
          <label>Web Base Path</label>
          <input type="text" value={webBasePath} onChange={e => setWebBasePath(e.target.value)} placeholder="/abcdefgh12345678" />
        </div>
        <div className="form-group">
          <label>Token</label>
          <input type="text" value={token} onChange={e => setToken(e.target.value)} placeholder="Bearer token" required />
        </div>
        <div className="form-group form-check">
          <label className="check-label">
            <input type="checkbox" checked={skipVerify} onChange={e => setSkipVerify(e.target.checked)} />
            Пропустить проверку TLS (self-signed)
          </label>
        </div>
        <div className="modal-actions">
          <button type="submit" className="btn btn-primary">➕ Добавить</button>
          <button type="button" className="btn" onClick={onClose}>Отмена</button>
        </div>
      </form>
    </Modal>
  );
}
