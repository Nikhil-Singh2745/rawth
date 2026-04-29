/* rawth Web UI — Terminal + Live Stats */
(function () {
  'use strict';

  let ws = null;
  let wsWasConnected = false;
  let commandHistory = [];
  let historyIndex = -1;
  const API_BASE = window.location.origin;
  const WS_URL = (window.location.protocol === 'https:' ? 'wss://' : 'ws://') + window.location.host + '/ws';

  const termOutput = document.getElementById('terminal-output');
  const termInput  = document.getElementById('terminal-input');
  const connStatus = document.getElementById('connection-status');
  const hexContent = document.getElementById('hex-content');

  document.addEventListener('DOMContentLoaded', () => {
    initHexDump();
    initTerminal();
    initHintButtons();
    initStatsPolling();
    initScrollAnimations();
    initFAQ();
  });

  // the hex dump in the hero. real rawth header bytes, give or take a few.
  function initHexDump() {
    if (!hexContent) return;
    const rawthHeader = [
      0x52, 0x41, 0x57, 0x54, 0x01, 0x00, 0x00, 0x00,
      0x00, 0x10, 0x00, 0x00, 0x0A, 0x00, 0x00, 0x00,
      0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
      0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
      0x02, 0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08,
      0x67, 0x72, 0x65, 0x65, 0x74, 0x69, 0x6E, 0x67,
      0x00, 0x31, 0x68, 0x65, 0x6C, 0x6C, 0x6F, 0x20,
      0x66, 0x72, 0x6F, 0x6D, 0x20, 0x72, 0x61, 0x77,
      0x74, 0x68, 0x06, 0x00, 0x02, 0x00, 0x61, 0x75,
      0x74, 0x68, 0x6F, 0x72, 0x6E, 0x69, 0x6B, 0x68,
      0x69, 0x6C, 0x08, 0x00, 0x02, 0x00, 0x6C, 0x61,
      0x6E, 0x67, 0x75, 0x61, 0x67, 0x65, 0x67, 0x6F,
    ];
    let html = '';
    for (let i = 0; i < rawthHeader.length; i += 16) {
      const offset = i.toString(16).padStart(8, '0');
      let hexStr = '', asciiStr = '';
      for (let j = 0; j < 16 && i + j < rawthHeader.length; j++) {
        const byte = rawthHeader[i + j];
        const magic = i === 0 && j < 4;
        const hx = byte.toString(16).padStart(2, '0');
        hexStr += magic ? `<span class="hex-magic">${hx}</span> ` : hx + ' ';
        const ch = byte >= 32 && byte < 127 ? String.fromCharCode(byte) : '.';
        asciiStr += magic ? `<span class="hex-magic">${ch}</span>` : ch;
        if (j === 7) hexStr += ' ';
      }
      html += `<div class="hex-line"><span class="hex-offset">${offset}</span><span class="hex-bytes">${hexStr}</span><span class="hex-ascii">${asciiStr}</span></div>`;
    }
    hexContent.innerHTML = html;
  }

  function initTerminal() {
    addTermLine('welcome to rawth — a database built from scratch', 'term-welcome');
    addTermLine('connecting to server...', 'term-info');
    connectWebSocket();

    termInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        const cmd = termInput.value.trim();
        if (!cmd) return;
        commandHistory.unshift(cmd);
        historyIndex = -1;
        executeCommand(cmd);
        termInput.value = '';
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        if (historyIndex < commandHistory.length - 1) {
          historyIndex++;
          termInput.value = commandHistory[historyIndex];
        }
      } else if (e.key === 'ArrowDown') {
        e.preventDefault();
        if (historyIndex > 0) {
          historyIndex--;
          termInput.value = commandHistory[historyIndex];
        } else {
          historyIndex = -1;
          termInput.value = '';
        }
      }
    });

    termOutput.addEventListener('click', () => termInput.focus());
  }

  function connectWebSocket() {
    try {
      ws = new WebSocket(WS_URL);
      ws.onopen = () => {
        wsWasConnected = true;
        setStatus('connected', 'connected');
        addTermLine('connected via WebSocket — type HELP to get started', 'term-ok');
      };
      ws.onmessage = (e) => {
        try {
          const result = JSON.parse(e.data);
          displayResult(result);
        } catch { addTermLine(e.data, 'term-info'); }
      };
      ws.onclose = () => {
        setStatus('disconnected', 'error');
        if (wsWasConnected) {
          addTermLine('WebSocket disconnected — falling back to HTTP', 'term-err');
        }
        ws = null;
      };
      ws.onerror = () => { setStatus('error', 'error'); ws = null; };
    } catch {
      setStatus('no WebSocket', 'error');
    }
  }

  function executeCommand(cmd) {
    addTermLine(`rawth> ${cmd}`, 'term-cmd');
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(cmd);
    } else {
      fetch(API_BASE + '/api/query', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query: cmd }),
      })
        .then((r) => r.json())
        .then((result) => displayResult(result))
        .catch((err) => addTermLine(`ERR: ${err.message}`, 'term-err'));
    }
  }

  function displayResult(result) {
    if (!result.ok) {
      addTermLine(result.message || 'unknown error', 'term-err');
      return;
    }
    if (result.value) addTermLine(`"${result.value}"`, 'term-val');
    if (result.message) addTermLine(result.message, 'term-ok');
    if (result.data) {
      if (Array.isArray(result.data)) {
        result.data.forEach((item, i) => addTermLine(`  ${i + 1}) ${item}`, 'term-info'));
      } else if (typeof result.data === 'object') {
        Object.entries(result.data).forEach(([k, v]) => addTermLine(`  ${k}: ${v}`, 'term-info'));
      }
    }
    fetchStats();
  }

  function addTermLine(text, className) {
    const line = document.createElement('div');
    line.className = `term-line ${className || ''}`;
    line.textContent = text;
    termOutput.appendChild(line);
    termOutput.scrollTop = termOutput.scrollHeight;
  }

  function setStatus(text, state) {
    const st = connStatus.querySelector('.status-text');
    if (st) st.textContent = text;
    connStatus.className = 'terminal-status ' + state;
  }

  // click hint → fill input, focus input. no scroll.
  function initHintButtons() {
    document.querySelectorAll('.hint-btn').forEach((btn) => {
      btn.addEventListener('click', () => {
        const cmd = btn.dataset.cmd;
        if (cmd) {
          termInput.value = cmd;
          termInput.focus();
        }
      });
    });
  }

  function initStatsPolling() {
    fetchStats();
    setInterval(fetchStats, 3000);
  }

  function fetchStats() {
    fetch(API_BASE + '/api/stats')
      .then((r) => r.json())
      .then((s) => updateStats(s))
      .catch(() => {});
  }

  function updateStats(s) {
    animateValue('stat-keys', s.key_count);
    animateValue('stat-depth', s.tree_depth);
    animateValue('stat-pages', s.page_count);
    animateValue('stat-puts', s.put_count);
    animateValue('stat-gets', s.get_count);
    animateValue('stat-deletes', s.delete_count);
    const sizeEl = document.getElementById('stat-size');
    if (sizeEl) sizeEl.textContent = formatBytes(s.file_size_bytes);
    const uptimeEl = document.getElementById('stat-uptime');
    if (uptimeEl) uptimeEl.textContent = s.uptime || '—';
  }

  function animateValue(id, value) {
    const el = document.getElementById(id);
    if (!el) return;
    const current = parseInt(el.textContent) || 0;
    if (current === value) return;
    el.textContent = value;
    el.style.transform = 'scale(1.12)';
    setTimeout(() => (el.style.transform = 'scale(1)'), 200);
  }

  function formatBytes(bytes) {
    if (!bytes) return '0 B';
    const k = 1024, sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  }

  // scroll-in animations for cards
  function initScrollAnimations() {
    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) {
          entry.target.style.opacity = '1';
          entry.target.style.transform = 'translateY(0)';
        }
      });
    }, { threshold: 0.08 });

    document.querySelectorAll('.arch-card, .stat-card, .flow-step').forEach((el) => {
      el.style.opacity = '0';
      el.style.transform = 'translateY(24px)';
      el.style.transition = 'opacity 0.5s ease, transform 0.5s ease';
      observer.observe(el);
    });
  }

  // faq accordion
  function initFAQ() {
    document.querySelectorAll('.faq-item').forEach((item) => {
      const btn = item.querySelector('.faq-q');
      if (!btn) return;
      btn.addEventListener('click', () => {
        const isOpen = item.classList.contains('open');
        // close all
        document.querySelectorAll('.faq-item.open').forEach((o) => o.classList.remove('open'));
        if (!isOpen) item.classList.add('open');
      });
    });
  }

})();
