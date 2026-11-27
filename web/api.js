export const $ = id => document.getElementById(id);

export function escapeHTML(value = '') {
  return String(value).replace(/[&<>"']/g, character => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  })[character]);
}

export async function api(path, options = {}) {
  const request = { ...options };
  if (request.body instanceof FormData) {
    // Materialize disk-backed files before WebView2's in-process resource bridge reads the body.
    // Keep the original boundary: Blob.type would lowercase it and break multipart parsing.
    const encoded = new Response(request.body);
    request.headers = { 'Content-Type': encoded.headers.get('Content-Type') };
    request.body = await encoded.arrayBuffer();
  } else if (request.body) {
    request.headers = { 'Content-Type': 'application/json' };
    request.body = JSON.stringify(request.body);
  }
  const response = await fetch(`/api${path}`, request);
  const text = await response.text();
  let result;
  try {
    result = JSON.parse(text);
  } catch {
    throw new Error(`本地服务返回了无法读取的响应（HTTP ${response.status}）`);
  }
  if (!response.ok) {
    const error = result.error || {};
    throw new Error([error.message, error.details].filter(Boolean).join('\n') || `HTTP ${response.status}`);
  }
  return result;
}

export function assetURL(sessionID, assetID, download = false) {
  return `/api/sessions/${encodeURIComponent(sessionID)}/assets/${encodeURIComponent(assetID)}${download ? '?download=1' : ''}`;
}

let toastTimer;
export function toast(message, isError = false) {
  clearTimeout(toastTimer);
  const container = document.querySelector('dialog[open]') || document.body;
  container.append($('toast'));
  $('toast').textContent = message;
  $('toast').classList.toggle('error', isError);
  $('toast').hidden = false;
  toastTimer = setTimeout(() => { $('toast').hidden = true; }, isError ? 6500 : 3500);
}

export function showError(id, message = '') {
  $(id).textContent = message;
  $(id).hidden = !message;
}

export function fileSize(bytes) {
  return bytes >= 1024 * 1024 ? `${(bytes / 1024 / 1024).toFixed(1)} MB` : `${Math.round(bytes / 1024)} KB`;
}
