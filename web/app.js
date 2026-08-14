'use strict';

const $ = (id) => document.getElementById(id);

const ICONS = {
  dir: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#38bdf8" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>',
  file: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#93a6c9" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/></svg>',
  dl: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M5 21h14"/></svg>',
  zip: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 8v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h6l3 3h7a2 2 0 0 1 2 2z"/><path d="M12 11v5"/><path d="m9.5 13.5 2.5 2.5 2.5-2.5"/></svg>',
  eye: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z"/><circle cx="12" cy="12" r="3"/></svg>',
  play: '<svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg>',
  pause: '<svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor"><path d="M6 5h4v14H6zM14 5h4v14h-4z"/></svg>',
  note: '<svg width="42" height="42" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>',
};

const state = {
  path: '/',
  entries: [],
  selected: new Set(),
  search: '',
  sortKey: 'name',
  sortAsc: true,
  info: null,
  auth: false,
  tasks: [],
  knownDone: new Set(),
  polling: null,
  refreshTimer: null,
  tickTimer: null,
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
    const err = new Error(msg);
    err.status = res.status;
    if (res.status === 401 && path !== '/api/login') showLogin();
    throw err;
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

/* ---------- 媒体类型 ---------- */
function extOf(name) {
  const i = name.lastIndexOf('.');
  return i < 0 ? '' : name.slice(i).toLowerCase();
}
const NATIVE_IMAGE = new Set(['.jpg', '.jpeg', '.png', '.gif', '.webp', '.svg', '.bmp', '.avif', '.ico', '.jfif']);
const NATIVE_VIDEO = new Set(['.mp4', '.m4v', '.webm', '.ogv', '.mov']);
const NATIVE_AUDIO = new Set(['.mp3', '.wav', '.ogg', '.oga', '.opus', '.flac', '.m4a', '.weba']);
const X_IMAGE = new Set(['.tif', '.tiff', '.heic', '.heif', '.jp2', '.ppm', '.pgm', '.pbm', '.pnm']);
const X_VIDEO = new Set(['.avi', '.mkv', '.flv', '.wmv', '.ts', '.m2ts', '.mts', '.mpg', '.mpeg', '.vob', '.3gp', '.asf', '.rm', '.rmvb', '.divx', '.mxf']);
const X_AUDIO = new Set(['.aac', '.ape', '.wma', '.mka', '.mid', '.midi', '.amr', '.caf', '.aiff', '.aif', '.ac3', '.ra', '.au', '.dts']);
function mediaKind(name) {
  const e = extOf(name);
  if (NATIVE_IMAGE.has(e) || X_IMAGE.has(e)) return 'image';
  if (NATIVE_VIDEO.has(e) || X_VIDEO.has(e)) return 'video';
  if (NATIVE_AUDIO.has(e) || X_AUDIO.has(e)) return 'audio';
  return '';
}
function isNativeMedia(name) {
  const e = extOf(name);
  return NATIVE_IMAGE.has(e) || NATIVE_VIDEO.has(e) || NATIVE_AUDIO.has(e);
}
function kindLabel(kind) { return { image: '图片预览', video: '视频播放', audio: '音频播放' }[kind] || ''; }
function transcodeAvailable() {
  return state.info && state.info.preview && state.info.preview.ffmpeg;
}

/* ---------- init ---------- */
async function init() {
  bindEvents();
  await bootstrap();
}

async function bootstrap() {
  try {
    state.info = await api('/api/info');
    state.auth = !!state.info.auth;
    hideLogin();
    renderTopmeta();
    const q = new URLSearchParams(location.search).get('path');
    await loadPath(q && q.startsWith('/') ? q : '/');
    if (!state.tickTimer) state.tickTimer = setInterval(tick, 1000);
    startPolling();
  } catch (err) {
    if (err && err.status === 401) { showLogin(); return; }
    hideLogin();
    toast('无法连接服务端', 'error');
  }
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
  $('login-form').addEventListener('submit', submitLogin);
  $('pv-close').addEventListener('click', closePreview);
  $('pv-backdrop').addEventListener('click', closePreview);
  document.addEventListener('keydown', (ev) => {
    if (ev.key === 'Escape' && !$('preview').classList.contains('hidden')) closePreview();
  });
  window.addEventListener('popstate', () => {
    const q = new URLSearchParams(location.search).get('path');
    loadPath(q && q.startsWith('/') ? q : '/', true);
  });
}

/* ---------- 登录 ---------- */
function showLogin() {
  const ov = $('login-overlay');
  ov.classList.remove('hidden');
  setTimeout(() => ov.classList.add('show'), 10);
  setTimeout(() => $('login-pass').focus(), 90);
}
function hideLogin() {
  const ov = $('login-overlay');
  ov.classList.remove('show');
  setTimeout(() => ov.classList.add('hidden'), 260);
  $('login-error').classList.remove('show');
  $('login-pass').value = '';
}
async function submitLogin(e) {
  e.preventDefault();
  const pass = $('login-pass').value;
  if (!pass) return;
  const btn = $('login-submit');
  btn.disabled = true;
  try {
    await api('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password: pass }),
    });
    await bootstrap();
  } catch (err) {
    const errEl = $('login-error');
    errEl.textContent = err.status === 401 ? '密码错误，请重试' : '登录失败：' + err.message;
    errEl.classList.add('show');
    const card = document.querySelector('.login-card');
    card.classList.remove('shake');
    void card.offsetWidth;
    card.classList.add('shake');
    $('login-pass').value = '';
    $('login-pass').focus();
  } finally {
    btn.disabled = false;
  }
}
async function logout() {
  try { await api('/api/logout', { method: 'POST' }); } catch (_) {}
  state.auth = false;
  showLogin();
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

  if (state.auth) {
    const out = document.createElement('span');
    out.className = 'chip clickable';
    out.id = 'btn-logout';
    out.title = '退出登录';
    out.textContent = '退出登录';
    out.addEventListener('click', logout);
    meta.appendChild(out);
  }

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
    const kind = mediaKind(e.name);
    if (kind) {
      const pv = document.createElement('a');
      pv.className = 'mini-btn pv';
      pv.href = 'javascript:void(0)';
      pv.title = '预览' + (isNativeMedia(e.name) ? '' : '（服务端转码）');
      pv.innerHTML = ICONS.eye;
      pv.addEventListener('click', (ev) => { ev.preventDefault(); ev.stopPropagation(); openPreview(e); });
      acts.appendChild(pv);
    }
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

/* ---------- 媒体预览 ---------- */
function openPreview(e) {
  const kind = mediaKind(e.name);
  if (!kind) return;
  const url = '/api/preview?path=' + encodeURIComponent(e.path);
  const body = $('pv-body');
  body.innerHTML = '';
  body.className = 'pv-body pv-' + kind;
  $('pv-title').textContent = e.name;
  $('pv-title').title = e.name;
  const kindEl = $('pv-kind');
  kindEl.textContent = kindLabel(kind);
  kindEl.className = 'pv-kind ' + kind;
  const foot = $('pv-foot');
  foot.innerHTML = '';
  const transcoded = !isNativeMedia(e.name) && transcodeAvailable();
  if (transcoded) {
    const hint = document.createElement('span');
    hint.className = 'pv-hint';
    hint.textContent = kind === 'video'
      ? '服务端正在转码播放，首次打开需等待片刻，且不支持拖动进度条'
      : '服务端正在转码播放，首次打开需等待片刻';
    foot.appendChild(hint);
  }
  const dl = document.createElement('a');
  dl.className = 'pv-dl';
  dl.href = '/api/download?path=' + encodeURIComponent(e.path);
  dl.download = e.name;
  dl.innerHTML = ICONS.dl + '下载原文件';
  foot.appendChild(dl);

  $('preview').classList.remove('hidden');
  document.body.classList.add('no-scroll');

  if (kind === 'image') renderImagePreview(url);
  else if (kind === 'video') renderVideoPreview(url);
  else renderAudioPreview(url, e);
}

function renderImagePreview(url) {
  const body = $('pv-body');
  const wrap = document.createElement('div');
  wrap.className = 'pv-img-wrap';
  const spinner = document.createElement('div');
  spinner.className = 'spinner';
  wrap.appendChild(spinner);
  const img = document.createElement('img');
  img.alt = '';
  img.addEventListener('load', () => spinner.remove());
  img.addEventListener('error', () => {
    spinner.remove();
    wrap.classList.add('pv-err');
    wrap.textContent = '图片加载失败，可点击下方“下载原文件”查看';
  });
  img.src = url;
  wrap.appendChild(img);
  body.appendChild(wrap);
}

function renderVideoPreview(url) {
  const body = $('pv-body');
  const wrap = document.createElement('div');
  wrap.className = 'pv-video-wrap';
  const v = document.createElement('video');
  v.controls = true;
  v.autoplay = true;
  v.playsInline = true;
  v.src = url;
  v.addEventListener('error', () => {
    wrap.classList.add('pv-err');
    wrap.textContent = '视频播放失败，可点击下方“下载原文件”后用本地播放器打开';
  });
  wrap.appendChild(v);
  body.appendChild(wrap);
}

function renderAudioPreview(url, e) {
  const body = $('pv-body');
  const card = document.createElement('div');
  card.className = 'audio-card';

  const art = document.createElement('div');
  art.className = 'audio-art';
  art.innerHTML = ICONS.note;

  const meta = document.createElement('div');
  meta.className = 'audio-meta';
  const name = document.createElement('div');
  name.className = 'audio-name';
  name.textContent = e.name;
  name.title = e.name;
  const size = document.createElement('div');
  size.className = 'audio-size';
  size.textContent = fmtSize(e.size) + ' · ' + (isNativeMedia(e.name) ? '浏览器原生格式' : '服务端转码格式');
  meta.append(name, size);

  const btn = document.createElement('button');
  btn.className = 'audio-play';
  btn.type = 'button';
  btn.title = '播放 / 暂停';
  btn.innerHTML = ICONS.play;

  const prog = document.createElement('div');
  prog.className = 'audio-progress';
  const pbar = document.createElement('div');
  pbar.className = 'audio-pbar';
  prog.appendChild(pbar);

  const times = document.createElement('div');
  times.className = 'audio-times';
  const cur = document.createElement('span');
  cur.textContent = '00:00';
  const dur = document.createElement('span');
  dur.textContent = '--:--';
  times.append(cur, dur);

  const vol = document.createElement('input');
  vol.className = 'audio-vol';
  vol.type = 'range';
  vol.min = 0;
  vol.max = 100;
  vol.value = 80;
  vol.title = '音量';

  const audio = document.createElement('audio');
  audio.preload = 'metadata';
  audio.src = url;

  let playing = false;
  const fmtT = (t) => {
    if (!isFinite(t) || t < 0) return '--:--';
    const m = Math.floor(t / 60), sec = Math.floor(t % 60);
    return String(m).padStart(2, '0') + ':' + String(sec).padStart(2, '0');
  };
  const syncBtn = () => { btn.innerHTML = playing ? ICONS.pause : ICONS.play; };
  btn.addEventListener('click', () => {
    if (playing) audio.pause();
    else audio.play().catch(() => toast('播放失败，可点击“下载原文件”查看', 'error'));
  });
  audio.addEventListener('play', () => { playing = true; syncBtn(); card.classList.add('playing'); });
  audio.addEventListener('pause', () => { playing = false; syncBtn(); card.classList.remove('playing'); });
  audio.addEventListener('ended', () => {
    playing = false; syncBtn(); card.classList.remove('playing');
    pbar.style.width = '0%'; cur.textContent = '00:00';
  });
  audio.addEventListener('loadedmetadata', () => { dur.textContent = fmtT(audio.duration); });
  audio.addEventListener('timeupdate', () => {
    cur.textContent = fmtT(audio.currentTime);
    if (isFinite(audio.duration) && audio.duration > 0) {
      pbar.style.width = (audio.currentTime / audio.duration * 100) + '%';
    }
  });
  prog.addEventListener('click', (ev) => {
    const r = prog.getBoundingClientRect();
    const ratio = Math.max(0, Math.min(1, (ev.clientX - r.left) / r.width));
    if (isFinite(audio.duration) && audio.duration > 0) {
      audio.currentTime = ratio * audio.duration;
    } else {
      toast('转码播放中，暂不支持拖动进度', 'info');
    }
  });
  vol.addEventListener('input', () => { audio.volume = vol.value / 100; });

  card.append(art, meta, btn, prog, times, vol);
  body.appendChild(card);
  body.appendChild(audio);
  audio.play().catch(() => {});
}

function closePreview() {
  const body = $('pv-body');
  body.querySelectorAll('video, audio').forEach((m) => {
    m.pause();
    m.removeAttribute('src');
    m.load();
  });
  body.innerHTML = '';
  body.className = 'pv-body';
  $('pv-foot').innerHTML = '';
  $('preview').classList.add('hidden');
  document.body.classList.remove('no-scroll');
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
