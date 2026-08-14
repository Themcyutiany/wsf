'use strict';

const $ = (id) => document.getElementById(id);

const ICONS = {
  dir: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#38bdf8" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>',
  file: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#93a6c9" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/></svg>',
  dl: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M5 21h14"/></svg>',
  zip: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 8v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h6l3 3h7a2 2 0 0 1 2 2z"/><path d="M12 11v5"/><path d="m9.5 13.5 2.5 2.5 2.5-2.5"/></svg>',
};

const state = {
  path: '/',
  entries: [],
  selected: new Set(),
  search: '',
  sortKey: 'name',
  sortAsc: true,
  info: null,
  tasks: [],
  knownDone: new Set(),
  polling: null,
  refreshTimer: null,
};

/* ---------- utils ---------- */
function fmtSize(n) {
  if (n == null || n < 0) return '—';
  if (n < 1024) return n + ' B';
  const units = ['KB', 'MB', 'GB', 'TB'];
  let v = n, i = -1;
  do { v /= 1024; i++; } while (v >= 1024 && i < units.length - 1);
  return v.toFixed(v >= 100 ? 0 : 1) + ' ' + units[i];
}
function fmtSpeed(n) {
  if (!n || n <= 0) return '';
  if (n >= 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB/s';
  if (n >= 1024) return (n / 1024).toFixed(1) + ' KB/s';
  return n + ' B/s';
}
function fmtTime(ts) {
  if (!ts) return '—';
  const d = new Date(ts);
  const p = (x) => String(x).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}
function fmtUptime(started) {
  const s = Math.max(0, Math.floor((Date.now() - new Date(started).getTime()) / 1000));
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60);
  if (h > 0) return `已运行 ${h} 小时 ${m} 分`;
  return `已运行 ${m} 分`;
}
async function api(path, opts) {
  const res = await fetch(path, opts);
  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try { const j = await res.json(); if (j.error) msg = j.error; } catch (_) {}
    throw new Error(msg);
  }
  return res.json();
}
function toast(msg, type = 'info') {
  const el = document.createElement('div');
  el.className = 'toast ' + type;
  el.textContent = msg;
  $('toasts').appendChild(el);
  setTimeout(() => el.remove(), 3600);
}
function escapeAttr(s) { return String(s).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;'); }

/* ---------- init ---------- */
async function init() {
  bindEvents();
  try {
    state.info = await api('/api/info');
    renderTopmeta();
  } catch (_) { toast('无法连接服务端', 'error'); }
  const q = new URLSearchParams(location.search).get('path');
  await loadPath(q && q.startsWith('/') ? q : '/');
  setInterval(tick, 1000);
  startPolling();
}

function bindEvents() {
  $('btn-refresh').addEventListener('click', () => loadPath(state.path, true));
  $('search').addEventListener('input', () => {
    state.search = $('search').value.trim().toLowerCase();
    renderList();
  });
  $('check-all').addEventListener('change', (e) => {
    state.selected.clear();
    if (e.target.checked) filteredSorted().forEach((it) => state.selected.add(it.path));
    renderList();
  });
  $('btn-zip').addEventListener('click', downloadSelected);
  $('btn-url').addEventListener('click', openUrlModal);
  $('modal-close').addEventListener('click', closeUrlModal);
  $('modal').addEventListener('click', (e) => { if (e.target === $('modal')) closeUrlModal(); });
  document.querySelectorAll('.col-sort').forEach((h) => {
    h.addEventListener('click', () => {
      const key = h.dataset.sort;
      if (state.sortKey === key) state.sortAsc = !state.sortAsc;
      else { state.sortKey = key; state.sortAsc = true; }
      renderList();
    });
  });
  $('url-form').addEventListener('submit', submitUrlDownload);
  window.addEventListener('popstate', () => {
    const q = new URLSearchParams(location.search).get('path');
    loadPath(q && q.startsWith('/') ? q : '/', true);
  });
}

/* ---------- info ---------- */
function renderTopmeta() {
  const meta = $('topmeta');
  meta.innerHTML = '';
  const root = document.createElement('span');
  root.className = 'chip';
  root.title = '共享目录';
  root.innerHTML = '共享目录 <b>' + escapeAttr(state.info.root) + '</b>';
  meta.appendChild(root);

  if (state.info.addrs && state.info.addrs.length) {
    const addr = document.createElement('span');
    addr.className = 'chip clickable';
    addr.title = '点击复制局域网访问地址';
    const url = `http://${state.info.addrs[0]}:${state.info.port}`;
    addr.innerHTML = '访问地址 <b>' + escapeAttr(url) + '</b>';
    addr.addEventListener('click', () => {
      navigator.clipboard?.writeText(url).then(
        () => toast('访问地址已复制', 'ok'),
        () => toast('复制失败', 'error')
      );
    });
    meta.appendChild(addr);
  }

  const proxy = document.createElement('span');
  proxy.className = 'chip';
  proxy.innerHTML = state.info.noProxy
    ? '远程下载 <b>直连</b>'
    : '远程下载代理 <b>' + escapeAttr(state.info.proxy) + '</b>';
  proxy.title = '“远程下载”功能通过该代理抓取链接，保存到共享目录';
  meta.appendChild(proxy);

  $('proxy-hint').textContent = state.info.noProxy
    ? '当前未启用代理，远程下载将直连目标服务器。'
    : `远程下载将通过代理 ${state.info.proxy} 抓取链接，下载完成后的文件会出现在共享目录中。`;
}

/* ---------- navigation ---------- */
async function loadPath(p, keep = false) {
  state.path = p || '/';
  if (!keep) {
    state.selected.clear();
    state.search = '';
    $('search').value = '';
    $('check-all').checked = false;
  }
  try {
    const data = await api('/api/list?path=' + encodeURIComponent(state.path));
    state.entries = data.entries;
    render();
    if (!keep) {
      const url = new URL(location.href);
      url.searchParams.set('path', state.path);
      history.pushState(null, '', url);
    }
  } catch (e) {
    toast('无法打开文件夹：' + e.message, 'error');
    if (!keep) { state.entries = []; render(); }
  }
}
function navigate(p) { loadPath(p); }

/* ---------- render ---------- */
function filteredSorted() {
  let list = state.entries;
  if (state.search) list = list.filter((e) => e.name.toLowerCase().includes(state.search));
  const key = state.sortKey, asc = state.sortAsc;
  const cmp = (a, b) => {
    if (key === 'size') return a.size - b.size;
    if (key === 'modTime') return new Date(a.modTime) - new Date(b.modTime);
    return a.name.localeCompare(b.name, 'zh-CN', { numeric: true });
  };
  const dirs = list.filter((e) => e.isDir).sort(cmp);
  const files = list.filter((e) => !e.isDir).sort(cmp);
  return [...(asc ? dirs : dirs.reverse()), ...(asc ? files : files.reverse())];
}

function render() {
  renderBreadcrumb();
  renderList();
}

function renderBreadcrumb() {
  const nav = $('breadcrumb');
  nav.innerHTML = '';
  const segs = state.path.split('/').filter(Boolean);
  const mk = (label, p, active) => {
    const el = document.createElement('span');
    el.className = 'crumb' + (active ? ' active' : '');
    el.textContent = label;
    if (!active) el.addEventListener('click', () => navigate(p));
    nav.appendChild(el);
  };
  mk('根目录', '/', segs.length === 0);
  segs.forEach((s, i) => {
    const sep = document.createElement('span');
    sep.className = 'crumb-sep';
    sep.textContent = '›';
    nav.appendChild(sep);
    mk(s, '/' + segs.slice(0, i + 1).join('/'), i === segs.length - 1);
  });
}

function renderList() {
  const list = $('list');
  list.innerHTML = '';
  const items = filteredSorted();
  const empty = $('empty');
  if (items.length === 0) {
    empty.classList.remove('hidden');
    $('empty-text').textContent = state.search ? '没有匹配的文件' : '此文件夹为空';
    $('summary').textContent = state.path === '/' ? '共享目录为空' : '该文件夹为空';
  } else {
    empty.classList.add('hidden');
    const dirs = items.filter((e) => e.isDir).length;
    let size = 0;
    items.forEach((e) => { if (!e.isDir) size += e.size; });
    $('summary').textContent = `共 ${items.length} 项 · ${dirs} 个文件夹 · 文件合计 ${fmtSize(size)}`;
  }

  const shown = items.length;
  $('check-all').checked = shown > 0 && state.selected.size === shown;
  $('btn-zip').disabled = state.selected.size === 0;
  ['name', 'size', 'modTime'].forEach((k) => {
    const el = $('arrow-' + k);
    el.textContent = state.sortKey === k ? (state.sortAsc ? '▲' : '▼') : '';
  });

  for (const e of items) list.appendChild(rowEl(e));
}

function rowEl(e) {
  const row = document.createElement('div');
  row.className = 'row' + (state.selected.has(e.path) ? ' selected' : '');

  const cbCell = document.createElement('div');
  cbCell.className = 'cell check';
  const cb = document.createElement('input');
  cb.type = 'checkbox';
  cb.checked = state.selected.has(e.path);
  cb.addEventListener('change', (ev) => {
    if (ev.target.checked) state.selected.add(e.path); else state.selected.delete(e.path);
    renderList();
  });
  cbCell.appendChild(cb);

  const nameCell = document.createElement('div');
  nameCell.className = 'cell name';
  const icon = document.createElement('span');
  icon.className = 'icon';
  icon.innerHTML = e.isDir ? ICONS.dir : ICONS.file;
  const a = document.createElement('a');
  a.className = 'name-link' + (e.hidden ? ' hidden-name' : '');
  a.textContent = e.name;
  a.title = e.name;
  if (e.isDir) {
    a.href = '?path=' + encodeURIComponent(e.path);
    a.addEventListener('click', (ev) => { ev.preventDefault(); navigate(e.path); });
  } else {
    a.href = '/api/download?path=' + encodeURIComponent(e.path);
    a.download = e.name;
    a.title += '（点击下载）';
  }
  nameCell.append(icon, a);

  const sizeCell = document.createElement('div');
  sizeCell.className = 'cell size';
  sizeCell.textContent = e.isDir ? '—' : fmtSize(e.size);
  sizeCell.title = e.isDir ? '文件夹' : fmtSize(e.size);

  const timeCell = document.createElement('div');
  timeCell.className = 'cell time';
  timeCell.textContent = fmtTime(e.modTime);

  const actCell = document.createElement('div');
  actCell.className = 'cell actions';
  const acts = document.createElement('div');
  acts.className = 'row-actions';
  if (e.isDir) {
    const zip = document.createElement('a');
    zip.className = 'mini-btn zip';
    zip.href = '/api/zip?path=' + encodeURIComponent(e.path);
    zip.title = '打包下载整个文件夹';
    zip.innerHTML = ICONS.zip + 'ZIP';
    acts.appendChild(zip);
  } else {
    const dl = document.createElement('a');
    dl.className = 'mini-btn dl';
    dl.href = '/api/download?path=' + encodeURIComponent(e.path);
    dl.download = e.name;
    dl.title = '下载';
    dl.innerHTML = ICONS.dl;
    acts.appendChild(dl);
  }
  actCell.appendChild(acts);

  row.append(cbCell, nameCell, sizeCell, timeCell, actCell);
  row.addEventListener('click', (ev) => {
    if (ev.target.closest('a, button, input')) return;
    cb.checked = !cb.checked;
    if (cb.checked) state.selected.add(e.path); else state.selected.delete(e.path);
    renderList();
  });
  return row;
}

async function downloadSelected() {
  const paths = [...state.selected];
  if (!paths.length) return;
  const btn = $('btn-zip');
  btn.disabled = true;
  const old = btn.textContent;
  btn.textContent = '打包中…';
  try {
    const res = await fetch('/api/zip', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ paths }),
    });
    if (!res.ok) {
      let msg = '打包失败';
      try { const j = await res.json(); if (j.error) msg = j.error; } catch (_) {}
      throw new Error(msg);
    }
    const blob = await res.blob();
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = paths.length === 1 ? (paths[0].split('/').pop() || '下载') + '.zip' : 'wsf-下载.zip';
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(a.href), 30000);
    toast(`已打包 ${paths.length} 项`, 'ok');
  } catch (e) {
    toast('打包失败：' + e.message, 'error');
  } finally {
    btn.disabled = false;
    btn.textContent = old;
  }
}

/* ---------- 远程下载 ---------- */
function openUrlModal() { $('modal').classList.remove('hidden'); refreshTasks(); }
function closeUrlModal() { $('modal').classList.add('hidden'); }

async function submitUrlDownload(e) {
  e.preventDefault();
  const url = $('url-input').value.trim();
  const filename = $('file-input').value.trim();
  if (!url) { toast('请输入链接', 'error'); return; }
  const btn = $('url-submit');
  btn.disabled = true;
  try {
    await api('/api/url-download', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url, filename }),
    });
    $('url-input').value = '';
    $('file-input').value = '';
    toast('已开始下载', 'ok');
    refreshTasks();
  } catch (err) {
    toast('启动失败：' + err.message, 'error');
  } finally {
    btn.disabled = false;
  }
}

async function refreshTasks() {
  try {
    const data = await api('/api/tasks');
    state.tasks = data.tasks || [];
    renderTasks();
    if (state.tasks.some((t) => t.status === 'downloading')) scheduleListRefresh(1500);
    for (const t of state.tasks) {
      if (t.status === 'done' && !state.knownDone.has(t.id)) {
        state.knownDone.add(t.id);
        toast(`下载完成：${t.filename || t.url}`, 'ok');
        scheduleListRefresh(600);
      }
    }
  } catch (_) {}
}

function renderTasks() {
  const box = $('task-list');
  box.innerHTML = '';
  $('task-empty').style.display = state.tasks.length ? 'none' : '';
  for (const t of state.tasks) {
    const el = document.createElement('div');
    el.className = 'task';

    const head = document.createElement('div');
    head.className = 't-head';
    const name = document.createElement('span');
    name.className = 't-name';
    name.textContent = t.filename || t.url;
    name.title = t.url;
    const status = document.createElement('span');
    status.className = 't-status ' + t.status;
    status.textContent = { downloading: '下载中', done: '完成', error: '失败', canceled: '已取消' }[t.status] || t.status;
    head.append(name, status);
    el.appendChild(head);

    if (t.status === 'downloading' || t.status === 'done') {
      const prog = document.createElement('div');
      prog.className = 'progress';
      const bar = document.createElement('div');
      bar.className = 'bar';
      bar.style.width = (t.status === 'done' ? 100 : (t.progress || 0)).toFixed(1) + '%';
      prog.appendChild(bar);
      el.appendChild(prog);
    }

    const meta = document.createElement('div');
    meta.className = 't-meta';
    if (t.status === 'downloading') {
      meta.textContent = `${fmtSize(t.downloaded)} / ${t.total > 0 ? fmtSize(t.total) : '未知'}${t.speed ? ' · ' + fmtSpeed(t.speed) : ''}`;
    } else if (t.status === 'done') {
      meta.textContent = `${fmtSize(t.downloaded)} · 已保存到共享目录`;
    }
    el.appendChild(meta);

    if (t.error) {
      const err = document.createElement('div');
      err.className = 't-err';
      err.textContent = t.error;
      el.appendChild(err);
    }

    const actions = document.createElement('div');
    actions.className = 't-meta';
    if (t.status === 'downloading') {
      const cancel = document.createElement('button');
      cancel.className = 'mini-btn';
      cancel.textContent = '取消';
      cancel.addEventListener('click', async () => {
        try { await api('/api/tasks/' + t.id + '/cancel', { method: 'POST' }); refreshTasks(); }
        catch (err) { toast(err.message, 'error'); }
      });
      actions.appendChild(cancel);
    } else if (t.status === 'done' && t.savedPath) {
      const link = document.createElement('a');
      link.className = 't-link';
      link.href = 'javascript:void(0)';
      link.textContent = '在共享目录查看';
      link.addEventListener('click', () => {
        const i = t.savedPath.lastIndexOf('/');
        const dir = i > 0 ? t.savedPath.slice(0, i) : '/';
        closeUrlModal();
        navigate(dir);
      });
      actions.appendChild(link);
    }
    el.appendChild(actions);
    box.appendChild(el);
  }
}

function scheduleListRefresh(ms = 800) {
  clearTimeout(state.refreshTimer);
  state.refreshTimer = setTimeout(() => {
    if (document.visibilityState !== 'hidden') loadPath(state.path, true).catch(() => {});
  }, ms);
}

function startPolling() {
  if (state.polling) return;
  state.polling = setInterval(() => {
    const modalOpen = !$('modal').classList.contains('hidden');
    const active = state.tasks.some((t) => t.status === 'downloading');
    if (modalOpen || active) refreshTasks();
  }, 2500);
}

function tick() {
  if (state.info) $('uptime').textContent = fmtUptime(state.info.startedAt);
}

init();
