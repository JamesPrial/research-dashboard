/* shared.js — API layer and utilities used by both dashboard and reader pages */

// --- Markdown rendering ---

marked.setOptions({ breaks: true });

function renderMarkdown(md) {
  return DOMPurify.sanitize(marked.parse(md));
}

// --- API layer ---

async function api(path, opts = {}) {
  const res = await fetch(path, opts);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status}: ${text}`);
  }
  return res;
}

async function apiJson(path, opts) {
  return (await api(path, opts)).json();
}

async function fetchList() {
  return apiJson('/research');
}

async function fetchJob(id) {
  return apiJson(`/research/${id}`);
}

async function fetchJobReport(id) {
  return (await api(`/research/${id}/report`)).text();
}

async function fetchPastReport(dirName) {
  return (await api(`/research/past/${dirName}/report`)).text();
}

async function startJob(query, model) {
  return apiJson('/research', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ query, model }),
  });
}

async function cancelJob(id) {
  return apiJson(`/research/${id}`, { method: 'DELETE' });
}

async function fetchFileList(dirName) {
  return apiJson(`/research/past/${dirName}/files`);
}

async function fetchFile(dirName, filePath) {
  return (await api(`/research/past/${dirName}/files/${filePath}`)).text();
}

async function fetchJobFileList(jobId) {
  return apiJson(`/research/${jobId}/files`);
}

async function fetchJobFile(jobId, filePath) {
  return (await api(`/research/${jobId}/files/${filePath}`)).text();
}

// --- Parsing helpers ---

function parseDirName(name) {
  // research-{topic}-{YYYYMMDD}-{HHMMSS}
  const m = name.match(/^research-(.+?)-(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})$/);
  if (!m) return { topic: name.replace(/^research-/, ''), date: '' };
  const topic = m[1];
  const d = new Date(+m[2], +m[3] - 1, +m[4], +m[5], +m[6], +m[7]);
  const date = d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  const time = d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
  return { topic, date: `${date}, ${time}` };
}

function truncate(s, n) {
  return s.length > n ? s.slice(0, n) + '...' : s;
}

// --- Formatting helpers ---

function formatElapsed(isoString) {
  const start = new Date(isoString);
  const now = new Date();
  const s = Math.floor((now - start) / 1000);
  if (s < 60) return s + 's';
  const m = Math.floor(s / 60);
  if (m < 60) return m + 'm';
  const h = Math.floor(m / 60);
  return h + 'h ' + (m % 60) + 'm';
}

function formatDuration(ms) {
  if (!ms) return '';
  const s = Math.round(ms / 1000);
  if (s < 60) return s + 's';
  const m = Math.floor(s / 60);
  const rem = s % 60;
  if (m < 60) return m + 'm ' + rem + 's';
  const h = Math.floor(m / 60);
  return h + 'h ' + (m % 60) + 'm';
}

function formatCost(usd) {
  if (usd == null) return '';
  if (usd < 0.01) return '<$0.01';
  return '$' + usd.toFixed(2);
}

function formatSize(bytes) {
  if (bytes == null) return '';
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
}

// --- Tool helpers ---

function getToolIcon(name) {
  const icons = {
    'Bash': '\u{1F4BB}',
    'Read': '\u{1F4C4}',
    'Write': '\u{270F}\uFE0F',
    'Edit': '\u{270F}\uFE0F',
    'Glob': '\u{1F50D}',
    'Grep': '\u{1F50D}',
    'WebSearch': '\u{1F310}',
    'WebFetch': '\u{1F310}',
    'Task': '\u{1F916}',
  };
  return icons[name] || '\u{1F527}';
}

function getToolPreview(name, input) {
  if (!input) return '';
  if (name === 'Bash' && input.command) return truncate(input.command, 60);
  if (name === 'Read' && input.file_path) return input.file_path.split('/').slice(-2).join('/');
  if ((name === 'Write' || name === 'Edit') && input.file_path) return input.file_path.split('/').slice(-2).join('/');
  if (name === 'Glob' && input.pattern) return input.pattern;
  if (name === 'Grep' && input.pattern) return input.pattern;
  if (name === 'WebSearch' && input.query) return truncate(input.query, 60);
  if (name === 'WebFetch' && input.url) return truncate(input.url, 60);
  if (name === 'Task' && input.description) return truncate(input.description, 60);
  // Generic: show first string value
  for (const v of Object.values(input)) {
    if (typeof v === 'string') return truncate(v, 50);
  }
  return '';
}

// --- DOM helpers ---

function escapeHtml(s) {
  const div = document.createElement('div');
  div.textContent = s;
  return div.innerHTML;
}

function escapeAttr(s) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
          .replace(/'/g, '&#39;').replace(/"/g, '&quot;');
}

// --- Source index parsing ---

function parseSourceIndex(indexMd) {
  if (!indexMd) return [];
  const lines = indexMd.split('\n');
  const sources = [];
  for (const line of lines) {
    const m = line.match(/^\|\s*(\d+)\s*\|\s*(.*?)\s*\|\s*(https?:\/\/\S+)\s*\|\s*\[md\]\((.*?)\)\s*\|\s*\[html\]\((.*?)\)\s*\|\s*(.*?)\s*\|$/);
    if (m) {
      sources.push({
        num: parseInt(m[1]),
        title: m[2].trim(),
        url: m[3].trim(),
        mdFile: m[4].trim(),
        htmlFile: m[5].trim(),
        status: m[6].trim(),
      });
    }
  }
  return sources;
}
