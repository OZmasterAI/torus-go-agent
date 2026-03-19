// === Torus Web UI ===

// --- State ---
const state = {
  messages: [],
  isStreaming: false,
  branch: null,
  model: null,
  provider: null,
  sessions: JSON.parse(localStorage.getItem('torus_sessions') || '[]'),
};

// --- DOM ---
const $ = (s) => document.querySelector(s);
const messagesEl    = $('#messages');
const welcomeEl     = $('#welcome');
const inputEl       = $('#input');
const sendBtn       = $('#btn-send');
const newBtn        = $('#btn-new');
const clearBtn      = $('#btn-clear');
const headerInfo    = $('#header-info');
const statusInfo    = $('#status-info');
const sessionList   = $('#session-list');
const sidebar       = $('#sidebar');
const sidebarToggle = $('#btn-sidebar-toggle');

// --- Configure marked ---
marked.setOptions({ breaks: true, gfm: true });

// --- API ---
async function fetchStatus() {
  const res = await fetch('/api/status');
  if (!res.ok) throw new Error('Status ' + res.status);
  return res.json();
}

async function createSession() {
  const res = await fetch('/api/new', { method: 'POST' });
  if (!res.ok) throw new Error('New session ' + res.status);
  return res.json();
}

async function clearCtx() {
  const res = await fetch('/api/clear', { method: 'POST' });
  if (!res.ok) throw new Error('Clear ' + res.status);
  return res.json();
}

// --- SSE streaming chat ---
async function sendChat(text) {
  const res = await fetch('/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message: text }),
  });
  if (!res.ok) throw new Error('Chat ' + res.status);

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const parts = buffer.split('\n\n');
    buffer = parts.pop();

    for (const part of parts) {
      for (const line of part.split('\n')) {
        if (line.startsWith('data: ')) {
          try { handleSSE(JSON.parse(line.slice(6))); }
          catch (e) { console.warn('SSE parse:', e); }
        }
      }
    }
  }
}

function handleSSE(data) {
  switch (data.type) {
    case 'start':
      state.isStreaming = true;
      addMessage('assistant', '');
      break;
    case 'delta':
      appendDelta(data.text);
      break;
    case 'done':
      state.isStreaming = false;
      finalizeMessage(data.text);
      break;
    case 'error':
      state.isStreaming = false;
      addMessage('error', data.error);
      break;
  }
}

// --- Messages ---
function hideWelcome() {
  if (welcomeEl) welcomeEl.style.display = 'none';
}

function addMessage(role, content) {
  hideWelcome();
  const msg = { role, content, ts: Date.now() };
  state.messages.push(msg);
  const idx = state.messages.length - 1;

  const el = document.createElement('div');
  el.className = 'message message-' + role;
  el.dataset.idx = idx;

  const label = document.createElement('div');
  label.className = 'message-label';
  label.textContent = role === 'user' ? 'You'
    : role === 'error' ? 'Error'
    : role === 'system' ? 'System'
    : 'Torus';

  const body = document.createElement('div');
  body.className = 'message-body';

  if (role === 'assistant' && content === '') {
    body.innerHTML = '<div class="thinking"><span></span><span></span><span></span></div>';
  } else if (role === 'error' || role === 'system') {
    body.textContent = content;
  } else {
    body.innerHTML = renderMd(content);
    postProcess(body);
  }

  el.appendChild(label);
  el.appendChild(body);
  messagesEl.appendChild(el);
  scrollToBottom();
}

function appendDelta(text) {
  const last = state.messages[state.messages.length - 1];
  if (!last || last.role !== 'assistant') return;
  last.content += text;

  const body = messagesEl.querySelector(
    '.message[data-idx="' + (state.messages.length - 1) + '"] .message-body'
  );
  if (!body) return;
  body.innerHTML = renderMd(last.content) + '<span class="cursor">\u2588</span>';
  scrollToBottom();
}

function finalizeMessage(fullText) {
  const last = state.messages[state.messages.length - 1];
  if (!last || last.role !== 'assistant') return;
  last.content = fullText;

  const body = messagesEl.querySelector(
    '.message[data-idx="' + (state.messages.length - 1) + '"] .message-body'
  );
  if (!body) return;
  body.innerHTML = renderMd(fullText);
  postProcess(body);
  scrollToBottom();
}

function renderMd(text) {
  if (!text) return '';
  return marked.parse(text);
}

// --- Syntax highlight + copy buttons ---
function postProcess(container) {
  container.querySelectorAll('pre code').forEach(function (block) {
    hljs.highlightElement(block);
  });
  container.querySelectorAll('pre').forEach(function (pre) {
    if (pre.querySelector('.copy-btn')) return;

    var code = pre.querySelector('code');
    var langMatch = code && code.className.match(/language-(\w+)/);
    if (langMatch) {
      var hdr = document.createElement('div');
      hdr.className = 'code-header';
      hdr.innerHTML = '<span>' + langMatch[1] + '</span>';
      pre.insertBefore(hdr, pre.firstChild);
    }

    var btn = document.createElement('button');
    btn.className = 'copy-btn';
    btn.textContent = 'Copy';
    btn.addEventListener('click', function () {
      var src = pre.querySelector('code');
      navigator.clipboard.writeText(src ? src.textContent : '').then(function () {
        btn.textContent = 'Copied!';
        setTimeout(function () { btn.textContent = 'Copy'; }, 2000);
      });
    });
    pre.appendChild(btn);
  });
}

// --- Scroll ---
var userScrolled = false;
messagesEl.addEventListener('scroll', function () {
  var el = messagesEl;
  userScrolled = el.scrollHeight - el.scrollTop - el.clientHeight > 80;
});

function scrollToBottom() {
  if (!userScrolled) {
    requestAnimationFrame(function () {
      messagesEl.scrollTop = messagesEl.scrollHeight;
    });
  }
}

// --- Input ---
inputEl.addEventListener('keydown', function (e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    handleSend();
  }
});

inputEl.addEventListener('input', function () {
  inputEl.style.height = 'auto';
  inputEl.style.height = Math.min(inputEl.scrollHeight, 200) + 'px';
});

sendBtn.addEventListener('click', handleSend);

async function handleSend() {
  var text = inputEl.value.trim();
  if (!text || state.isStreaming) return;

  inputEl.value = '';
  inputEl.style.height = 'auto';
  addMessage('user', text);

  sendBtn.disabled = true;
  try {
    await sendChat(text);
  } catch (err) {
    if (state.isStreaming) {
      state.isStreaming = false;
      var last = state.messages[state.messages.length - 1];
      if (last && last.role === 'assistant' && last.content) {
        finalizeMessage(last.content);
      }
    }
    addMessage('error', err.message);
  }
  sendBtn.disabled = false;
  inputEl.focus();
}

// --- Sessions ---
newBtn.addEventListener('click', async function () {
  if (state.isStreaming) return;
  try {
    var data = await createSession();
    state.messages = [];
    messagesEl.innerHTML = '';
    if (welcomeEl) { messagesEl.appendChild(welcomeEl); welcomeEl.style.display = ''; }

    var session = {
      id: data.branch,
      name: 'Session ' + (state.sessions.length + 1),
      createdAt: Date.now(),
    };
    state.sessions.push(session);
    state.branch = data.branch;
    saveSessions();
    renderSessions();
    renderHeader();
  } catch (err) {
    addMessage('error', 'Failed to create session: ' + err.message);
  }
});

clearBtn.addEventListener('click', async function () {
  if (state.isStreaming) return;
  try {
    await clearCtx();
    state.messages = [];
    messagesEl.innerHTML = '';
    if (welcomeEl) { messagesEl.appendChild(welcomeEl); welcomeEl.style.display = ''; }
  } catch (err) {
    addMessage('error', 'Failed to clear: ' + err.message);
  }
});

function saveSessions() {
  localStorage.setItem('torus_sessions', JSON.stringify(state.sessions));
}

function renderSessions() {
  sessionList.innerHTML = '';
  var reversed = state.sessions.slice().reverse();
  for (var i = 0; i < reversed.length; i++) {
    var s = reversed[i];
    var el = document.createElement('div');
    el.className = 'session-item' + (s.id === state.branch ? ' active' : '');
    el.innerHTML = '<span class="session-dot"></span><span class="session-name">' + esc(s.name) + '</span>';
    sessionList.appendChild(el);
  }
}

// --- Header ---
function renderHeader() {
  var model = state.model || '...';
  var branch = state.branch ? state.branch.slice(0, 12) : '...';
  headerInfo.innerHTML = '<span style="color:var(--accent)">' + esc(model) + '</span> &middot; <span>' + esc(branch) + '</span>';
}

// --- Sidebar toggle ---
sidebarToggle.addEventListener('click', function () {
  sidebar.classList.toggle('open');
});
document.addEventListener('click', function (e) {
  if (sidebar.classList.contains('open') && !sidebar.contains(e.target) && e.target !== sidebarToggle) {
    sidebar.classList.remove('open');
  }
});

// --- Utilities ---
function esc(s) {
  var d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

// --- Init ---
async function init() {
  try {
    var status = await fetchStatus();
    state.branch = status.branch;
    state.model = status.model;
    state.provider = status.provider;

    if (!state.sessions.find(function (s) { return s.id === status.branch; })) {
      state.sessions.push({
        id: status.branch,
        name: 'Session ' + (state.sessions.length + 1),
        createdAt: Date.now(),
      });
      saveSessions();
    }

    renderSessions();
    renderHeader();
    statusInfo.innerHTML =
      '<div class="status-row"><span class="status-label">Provider</span><span>' + esc(status.provider) + '</span></div>' +
      '<div class="status-row"><span class="status-label">Model</span><span>' + esc(status.model) + '</span></div>' +
      '<div class="status-row"><span class="status-label">Messages</span><span>' + status.messages + '</span></div>';
  } catch (err) {
    headerInfo.textContent = 'Disconnected';
    addMessage('error', 'Cannot connect to server. Is the agent running with --http?');
  }
  inputEl.focus();
}

init();
