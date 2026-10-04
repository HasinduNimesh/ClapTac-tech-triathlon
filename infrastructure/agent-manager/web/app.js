// Viewer for the agent manager. Everything from the API is untrusted text (reasons can
// contain model output), so it is only ever written with textContent, never innerHTML.
(() => {
  const $ = (id) => document.getElementById(id);
  const TOKEN_KEY = 'agent-manager-view-token';
  let token = '';
  try { token = sessionStorage.getItem(TOKEN_KEY) || ''; } catch (_) { /* storage may be blocked */ }
  let selected = location.hash.slice(1);

  function el(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  async function api(path) {
    const res = await fetch(path, { headers: token ? { Authorization: 'Bearer ' + token } : {} });
    if (res.status === 401) { showLogin(token ? 'Wrong token.' : ''); throw new Error('unauthorized'); }
    if (!res.ok) throw new Error(res.statusText);
    $('login').hidden = true;
    return res.json();
  }

  function showLogin(message) {
    $('login').hidden = false;
    $('loginError').textContent = message;
  }

  const fmtTime = (iso) => new Date(iso).toLocaleString();
  const fmtMs = (ms) => (ms >= 1000 ? (ms / 1000).toFixed(1) + ' s' : ms + ' ms');

  async function loadList() {
    const p = new URLSearchParams({ limit: '200' });
    for (const k of ['agent', 'status', 'q']) if ($(k).value) p.set(k, $(k).value);
    const data = await api('/api/v1/traces?' + p);

    const agent = $('agent');
    const current = agent.value;
    agent.replaceChildren(new Option('All agents', ''), ...data.agents.map((a) => new Option(a, a)));
    agent.value = current;

    const rows = $('rows');
    rows.replaceChildren();
    $('empty').hidden = data.items.length > 0;
    for (const t of data.items) {
      const tr = el('tr', t.id === selected ? 'selected' : '');
      tr.tabIndex = 0;
      tr.append(el('td', '', fmtTime(t.started_at)), el('td', '', t.agent), el('td', '', t.operation),
        cell(badge(t.status)), el('td', '', String(t.steps)), el('td', '', fmtMs(t.duration_ms)));
      const open = () => select(t.id);
      tr.addEventListener('click', open);
      tr.addEventListener('keydown', (e) => { if (e.key === 'Enter') open(); });
      rows.append(tr);
    }
  }

  const cell = (child) => { const td = el('td'); td.append(child); return td; };
  const badge = (status) => el('span', 'badge ' + status, status.replace('_', ' '));

  async function select(id) {
    selected = id;
    location.hash = id;
    for (const r of $('rows').children) r.classList.remove('selected');
    await loadList();
    renderDetail(await api('/api/v1/traces/' + encodeURIComponent(id)));
  }

  function renderDetail(t) {
    const box = $('detail');
    box.replaceChildren();
    box.append(el('h2', '', t.agent + ' · ' + t.operation), badge(t.status));
    const meta = el('dl', 'meta');
    const add = (k, v) => { if (v) { meta.append(el('dt', '', k), el('dd', '', String(v))); } };
    add('Started', fmtTime(t.started_at));
    add('Took', fmtMs(new Date(t.ended_at) - new Date(t.started_at)));
    add('User', t.actor_id);
    add('Request id', t.correlation_id);
    add('Related', t.ref);
    add('Input', t.input_chars ? t.input_chars + ' characters (sha256 ' + (t.input_hash || '').slice(0, 12) + '…, text not stored)' : '');
    box.append(meta);

    const list = el('ol', 'steps');
    for (const s of t.steps || []) {
      const li = el('li', 'step ' + s.kind);
      const head = el('div', 'head');
      head.append(el('span', 'kind', s.kind), el('strong', '', s.name));
      if (s.outcome) head.append(el('span', 'outcome', s.outcome));
      if (s.duration_ms) head.append(el('span', 'muted', fmtMs(s.duration_ms)));
      li.append(head, el('p', 'reason', s.reason || 'No reason recorded.'));
      const keys = Object.keys(s.detail || {});
      if (keys.length) {
        const d = el('details');
        d.append(el('summary', '', 'Details'));
        const dl = el('dl', 'meta');
        for (const k of keys) dl.append(el('dt', '', k), el('dd', '', String(s.detail[k])));
        d.append(dl);
        li.append(d);
      }
      list.append(li);
    }
    box.append(el('h3', '', 'What it did, and why'), list);
  }

  let timer;
  function schedule() {
    clearInterval(timer);
    if ($('auto').checked) timer = setInterval(() => loadList().catch(() => {}), 5000);
  }

  $('login').addEventListener('submit', (e) => {
    e.preventDefault();
    token = $('token').value;
    try { sessionStorage.setItem(TOKEN_KEY, token); } catch (_) { /* ignore */ }
    loadList().catch(() => {});
  });
  for (const id of ['agent', 'status']) $(id).addEventListener('change', () => loadList().catch(() => {}));
  $('q').addEventListener('input', () => loadList().catch(() => {}));
  $('refresh').addEventListener('click', () => loadList().catch(() => {}));
  $('auto').addEventListener('change', schedule);

  loadList().then(() => { if (selected) select(selected).catch(() => {}); }).catch(() => {});
  schedule();
})();
