(() => {
  'use strict';

  const $ = (id) => document.getElementById(id);
  const VIEWS = ['loading', 'signin', 'unavailable', 'app'];

  function show(name) {
    for (const v of VIEWS) $('view-' + v).hidden = v !== name;
    $('signout').hidden = name !== 'app';
  }

  async function api(method, path, body) {
    const res = await fetch(path, {
      method,
      credentials: 'same-origin',
      headers: body ? { 'Content-Type': 'application/json' } : {},
      body: body ? JSON.stringify(body) : undefined,
    });
    let data = null;
    if (res.status !== 204) {
      try { data = await res.json(); } catch { /* non-JSON error body */ }
    }
    return { status: res.status, ok: res.ok, data };
  }

  function setError(msg) {
    $('error').textContent = msg || '';
    $('error').hidden = !msg;
  }

  // Remove the private key from the page as soon as the user is done.
  function clearResult() {
    $('config').textContent = '';
    $('qr').removeAttribute('src');
    $('result').hidden = true;
  }

  async function loadBrand() {
    const r = await api('GET', '/api/v1/brand');
    if (!r.ok || !r.data) return;
    document.title = r.data.name;
    $('brand-name').textContent = r.data.name;
    document.documentElement.style.setProperty('--brand', r.data.color);
    if (r.data.logoUrl) {
      $('brand-logo').src = r.data.logoUrl;
      $('brand-logo').hidden = false;
    }
  }

  function cell(text) {
    const td = document.createElement('td');
    td.textContent = text;
    return td;
  }

  function renderPeers(peers) {
    const body = $('peers').querySelector('tbody');
    body.replaceChildren();
    for (const p of peers) {
      const tr = document.createElement('tr');
      tr.append(
        cell(p.name),
        cell(p.address),
        cell(new Date(p.createdAt).toLocaleString()),
        cell(p.expiresAt ? new Date(p.expiresAt).toLocaleString() : 'Never'),
      );
      const actions = document.createElement('td');
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'link';
      btn.textContent = 'Remove';
      btn.setAttribute('aria-label', 'Remove ' + p.name);
      btn.addEventListener('click', () => removePeer(p));
      actions.append(btn);
      tr.append(actions);
      body.append(tr);
    }
    $('peers').hidden = peers.length === 0;
    $('empty').hidden = peers.length !== 0;
  }

  async function refresh() {
    const r = await api('GET', '/api/v1/me');
    if (r.status === 401) return show('signin');
    if (!r.ok || !r.data) return show('unavailable');
    $('user-name').textContent = r.data.name;
    renderPeers(r.data.peers || []);
    show('app');
  }

  async function removePeer(p) {
    if (!window.confirm('Remove "' + p.name + '"? The device will lose access.')) return;
    setError('');
    clearResult();
    const r = await api('DELETE', '/api/v1/me/peers/' + encodeURIComponent(p.id));
    if (!r.ok) setError((r.data && r.data.error) || 'Could not remove the device.');
    await refresh();
  }

  // wg-quick uses the file name as the interface name: max 15 chars, no spaces.
  function fileName(name) {
    const base = name.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 15);
    return (base || 'vpn') + '.conf';
  }

  function download(config, name) {
    const url = URL.createObjectURL(new Blob([config], { type: 'text/plain' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = fileName(name);
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  async function createPeer(ev) {
    ev.preventDefault();
    setError('');
    clearResult();
    const name = $('device-name').value.trim();
    const r = await api('POST', '/api/v1/me/peers', { name });
    if (!r.ok) {
      setError((r.data && r.data.error) || 'Could not create the device.');
      return;
    }
    $('device-name').value = '';
    $('config').textContent = r.data.config;
    if (r.data.qrSvg) {
      $('qr').src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(r.data.qrSvg);
      $('qr').hidden = false;
    } else {
      $('qr').hidden = true;
    }
    $('download').onclick = () => download(r.data.config, r.data.peer.name);
    $('result').hidden = false;
    await refresh();
  }

  $('create').addEventListener('submit', createPeer);
  $('done').addEventListener('click', clearResult);
  $('signout').addEventListener('click', async () => {
    clearResult();
    await api('POST', '/auth/logout');
    await refresh();
  });

  loadBrand();
  refresh().catch(() => show('unavailable'));
})();
