import { $, api, toast, showError } from './api.js';
import { hydrateIcons } from './icons.js';
import { renderHistory, renderConversation, renderAttachments, renderModels, modelConfigCard } from './views.js';
import { createViewer, copyImage } from './viewer.js';
import { initializeControls, refreshControls } from './controls.js';
import { initializeUpdates } from './updates.js';

hydrateIcons();
initializeControls();
$('local-address').textContent = location.host;

const state = { session: null, sessions: [], models: [], references: [], layerMode: false, busy: false };
const drafts = new Map();
let pollTimer;
let navigationVersion = 0;

function options() {
  return {
    count: state.layerMode ? 1 : Number($('count').value),
    resolution: $('resolution').value, ratio: $('ratio').value,
    background: $('background').value, format: $('format').value,
    watermark: $('watermark').checked, layer_decomposition: state.layerMode,
    max_images: Number($('max-images').value), text_recognition: $('text-recognition').checked,
  };
}

function applyOptions(value = {}) {
  $('count').value = value.count || 1;
  $('resolution').value = value.resolution || 'auto';
  $('ratio').value = value.ratio || 'auto';
  $('background').value = value.background || 'auto';
  $('format').value = value.format || 'png';
  $('watermark').checked = value.watermark || false;
  $('max-images').value = value.max_images || 30;
  $('text-recognition').checked = value.text_recognition ?? true;
  setLayerMode(value.layer_decomposition || false);
}

function setLayerMode(enabled) {
  state.layerMode = enabled;
  $('composer-mode').hidden = !enabled;
  $('layer-options').hidden = !enabled;
  $('count').disabled = enabled;
  $('ratio').disabled = enabled;
  $('background').disabled = enabled;
  $('enable-layers').hidden = enabled;
  $('prompt').placeholder = enabled
    ? '可选：描述要拆出的元素；留空让模型自动识别…'
    : '描述你脑海中的画面，或上传图片继续创作…';
  updateControls();
}

function updateControls() {
  refreshControls();
  const running = state.session?.turns.some(turn => turn.status === 'running') || false;
  $('generate-button').hidden = running;
  $('stop-button').hidden = !running;
  $('generate-button').disabled = state.busy;
  $('upload-button').disabled = state.busy;
  $('generate-button').querySelector('span').textContent = state.busy ? '保存中' : state.layerMode ? '拆分' : '生成';
  const configured = state.models.find(model => model.id === $('model').value)?.has_key;
  $('composer-hint').textContent = !configured ? '首次使用，请在右上角配置 API Key'
    : state.layerMode ? '选择 1 张 PNG / JPEG 原图 · 支持文字 OCR'
    : state.references.length ? `${state.references.length} 张参考图 · 点击缩略图可局部编辑`
    : '支持拖入或粘贴参考图';
}

function saveDraft() {
  drafts.set(state.session?.id || 'new', {
    prompt: $('prompt').value, references: [...state.references], options: options(), model: $('model').value,
  });
}

function restoreDraft(id) {
  const draft = drafts.get(id || 'new');
  $('prompt').value = draft?.prompt || '';
  state.references = draft?.references || [];
  if (draft?.model && state.models.some(model => model.id === draft.model)) $('model').value = draft.model;
  applyOptions(draft?.options);
  renderAttachments(state.session, state.references);
  showError('composer-error');
  resizePrompt();
}

async function refreshHistory() {
  state.sessions = await api('/sessions');
  renderHistory(state.sessions, state.session?.id);
}

function schedulePoll() {
  clearTimeout(pollTimer);
  if (!state.sessions.some(session => session.running)) return;
  pollTimer = setTimeout(async () => {
    try {
      const version = navigationVersion;
      const currentID = state.session?.id;
      await refreshHistory();
      if (currentID) {
        const session = await api(`/sessions/${currentID}`);
        if (version === navigationVersion) {
          state.session = session;
          renderConversation(session);
          updateControls();
        }
      }
      schedulePoll();
    } catch (error) {
      toast(`暂时无法读取生成进度：${error.message}。刷新页面可重新查看。`, true);
    }
  }, 1000);
}

async function openSession(id) {
  if (state.busy) { toast('图片正在保存，请稍后切换任务'); return; }
  saveDraft();
  const version = ++navigationVersion;
  try {
    const session = id ? await api(`/sessions/${id}`) : null;
    if (version !== navigationVersion) return;
    state.session = session;
    history.replaceState(null, '', id ? `/?session=${encodeURIComponent(id)}` : '/');
    restoreDraft(id);
    renderConversation(session, true);
    renderHistory(state.sessions, id);
    $('sidebar').classList.remove('open');
    $('prompt').focus();
  } catch (error) { toast(error.message, true); }
}

async function ensureSession() {
  if (state.session) return state.session;
  const session = await api('/sessions', { method: 'POST' });
  state.session = session;
  drafts.delete('new');
  history.replaceState(null, '', `/?session=${session.id}`);
  await refreshHistory();
  return session;
}

function addReference(asset) {
  if (state.references.includes(asset.id)) return true;
  if (state.references.length === 10) {
    toast('Seedream 5.5 每次最多使用 10 张参考图，请先移除一张。', true);
    return false;
  }
  state.references.push(asset.id);
  renderAttachments(state.session, state.references);
  updateControls();
  return true;
}

function editImage(asset) {
  setLayerMode(false);
  if (!addReference(asset)) return false;
  $('prompt').placeholder = '描述这次要修改的内容，也可以说明希望保留的细节…';
  $('prompt').focus();
  return true;
}

function decomposeImage(asset) {
  state.references = [asset.id];
  $('prompt').value = '';
  setLayerMode(true);
  renderAttachments(state.session, state.references);
  $('prompt').focus();
}

const viewer = createViewer({
  onEdit: editImage,
  onLayers: decomposeImage,
  onSelection(asset, token) {
    if (!editImage(asset)) return false;
    const number = state.references.indexOf(asset.id) + 1;
    const input = $('prompt');
    const position = input.selectionStart;
    input.setRangeText(`图 ${number} ${token} `, position, input.selectionEnd, 'end');
    input.focus();
    resizePrompt();
    return true;
  },
});

async function uploadFiles(files) {
  if (!files.length || state.busy) return;
  if (state.references.length + files.length > 10) {
    showError('composer-error', 'Seedream 5.5 每次最多接受 10 张参考图。');
    return;
  }
  state.busy = true;
  updateControls();
  showError('composer-error');
  try {
    const session = await ensureSession();
    for (const file of files) {
      const body = new FormData();
      body.append('file', file);
      const asset = await api(`/sessions/${session.id}/assets`, { method: 'POST', body });
      state.session.assets.push(asset);
      addReference(asset);
    }
    await refreshHistory();
  } catch (error) { showError('composer-error', error.message); }
  finally {
    state.busy = false;
    $('file-input').value = '';
    updateControls();
  }
}

async function generate(event) {
  event.preventDefault();
  if (state.busy) return;
  if (!state.models.find(model => model.id === $('model').value)?.has_key) {
    openSettings();
    showError('settings-error', '请填写 API Key 并保存，然后开始创作。');
    return;
  }
  const input = {
    model_id: $('model').value, prompt: $('prompt').value,
    reference_ids: [...state.references], options: options(),
  };
  if (!input.prompt.trim() && !state.layerMode) { $('prompt').focus(); return; }
  state.busy = true;
  updateControls();
  showError('composer-error');
  try {
    const session = await ensureSession();
    state.session = await api(`/sessions/${session.id}/turns`, { method: 'POST', body: input });
    $('prompt').value = '';
    state.references = [];
    saveDraft();
    renderAttachments(state.session, state.references);
    renderConversation(state.session, true);
    resizePrompt();
    await refreshHistory();
    schedulePoll();
  } catch (error) { showError('composer-error', error.message); }
  finally { state.busy = false; updateControls(); }
}

function resizePrompt() {
  const input = $('prompt');
  input.style.height = 'auto';
  input.style.height = `${input.scrollHeight}px`;
}

function openSettings() {
  $('model-configs').replaceChildren(...state.models.map(modelConfigCard));
  showError('settings-error');
  $('drawer-scrim').hidden = false;
  $('settings-drawer').classList.add('open');
  $('settings-drawer').inert = false;
  $('settings-drawer').setAttribute('aria-hidden', 'false');
  $('settings-close').focus();
}

function closeSettings() {
  $('drawer-scrim').hidden = true;
  $('settings-drawer').classList.remove('open');
  $('settings-drawer').inert = true;
  $('settings-drawer').setAttribute('aria-hidden', 'true');
  $('settings-open').focus();
}

async function saveSettings(event) {
  event.preventDefault();
  const models = Array.from($('model-configs').children).map(card => ({
    id: card.dataset.id,
    name: card.querySelector('[name=name]').value,
    model_name: card.querySelector('[name=model_name]').value,
    api_key: card.querySelector('[name=api_key]').value,
    url: card.querySelector('[name=url]').value,
  }));
  $('save-settings').disabled = true;
  showError('settings-error');
  try {
    const config = await api('/config', { method: 'PUT', body: { models } });
    state.models = config.models;
    renderModels(state.models, $('model').value);
    closeSettings();
    $('model-configs').replaceChildren();
    updateControls();
    toast('模型配置已保存');
  } catch (error) { showError('settings-error', error.message); }
  finally { $('save-settings').disabled = false; }
}

function handleImageAction(event) {
  const button = event.target.closest('[data-view], [data-edit], [data-copy], [data-reuse]');
  if (!button || !state.session) return;
  if (button.dataset.reuse) {
    const turn = state.session.turns.find(turn => turn.id === button.dataset.reuse);
    $('prompt').value = turn.prompt;
    state.references = [...(turn.reference_ids || [])];
    if (state.models.some(model => model.id === turn.model_id)) $('model').value = turn.model_id;
    applyOptions(turn.options);
    renderAttachments(state.session, state.references);
    resizePrompt();
    $('prompt').focus();
    return;
  }
  const assetID = button.dataset.view || button.dataset.edit || button.dataset.copy;
  const asset = state.session.assets.find(asset => asset.id === assetID);
  if (!asset) return;
  if (button.dataset.view) viewer.open(state.session, asset);
  if (button.dataset.edit) editImage(asset);
  if (button.dataset.copy) copyImage(state.session.id, asset).catch(error => toast(error.message, true));
}

$('composer').addEventListener('submit', generate);
$('prompt').addEventListener('input', resizePrompt);
$('new-task').onclick = () => openSession(null);
$('history').onclick = event => {
  const button = event.target.closest('[data-session]');
  if (button) openSession(button.dataset.session);
};
$('conversation').addEventListener('click', handleImageAction);
$('attachments').addEventListener('click', event => {
  handleImageAction(event);
  const button = event.target.closest('[data-remove]');
  if (!button) return;
  const removedNumber = state.references.indexOf(button.dataset.remove) + 1;
  state.references = state.references.filter(id => id !== button.dataset.remove);
  $('prompt').value = $('prompt').value.replace(/图\s*(\d+)\s*<(point|bbox)>[^<]*<\/\2>/g, (token, number) => {
    if (Number(number) === removedNumber) return '';
    return Number(number) > removedNumber ? token.replace(/图\s*\d+/, `图 ${Number(number) - 1}`) : token;
  });
  renderAttachments(state.session, state.references);
  updateControls();
});
$('upload-button').onclick = () => $('file-input').click();
$('file-input').onchange = event => uploadFiles([...event.target.files]);
$('image-starter').onclick = () => $('file-input').click();
$('text-starter').onclick = () => $('prompt').focus();
$('layer-starter').onclick = () => { setLayerMode(true); $('file-input').click(); };
$('enable-layers').onclick = () => setLayerMode(true);
$('exit-layers').onclick = () => setLayerMode(false);
$('advanced-toggle').onclick = () => {
  $('advanced-options').hidden = !$('advanced-options').hidden;
  $('advanced-toggle').setAttribute('aria-expanded', String(!$('advanced-options').hidden));
};
$('background').onchange = () => {
  if ($('background').value === 'transparent') $('format').value = 'png';
  refreshControls();
};
$('model').onchange = () => { localStorage.setItem('pureagent-model', $('model').value); updateControls(); };
$('stop-button').onclick = async () => {
  try {
    await api(`/sessions/${state.session.id}/cancel`, { method: 'POST' });
    toast('正在停止，已生成的图片会保留');
  } catch (error) { toast(error.message, true); }
};
$('settings-open').onclick = openSettings;
$('settings-close').onclick = closeSettings;
$('drawer-scrim').onclick = closeSettings;
$('settings-form').addEventListener('submit', saveSettings);
$('add-model').onclick = () => {
  const card = modelConfigCard();
  $('model-configs').append(card);
  card.querySelector('input').focus();
};
$('model-configs').onclick = event => {
  const button = event.target.closest('.remove-model');
  if (button) button.closest('.model-config').remove();
};
$('menu-button').onclick = () => $('sidebar').classList.toggle('open');
document.addEventListener('click', event => {
  if (!event.target.closest('#sidebar, #menu-button')) $('sidebar').classList.remove('open');
});

document.addEventListener('keydown', event => {
  if (event.key === 'Escape' && $('settings-drawer').classList.contains('open')) closeSettings();
  if ((event.ctrlKey || event.metaKey) && event.key === 'Enter' && !$('image-dialog').open && !$('settings-drawer').classList.contains('open')) {
    event.preventDefault();
    if (!$('generate-button').hidden) $('composer').requestSubmit();
  }
  if (event.key.toLowerCase() === 'n' && !event.ctrlKey && !event.metaKey && !event.altKey && !event.target.closest('input, textarea, select, [role=combobox], dialog, [contenteditable=true]') && !$('settings-drawer').classList.contains('open')) {
    event.preventDefault();
    openSession(null);
  }
});

document.addEventListener('paste', event => {
  if (event.target.closest('#settings-drawer, #image-dialog')) return;
  const files = [...(event.clipboardData?.files || [])];
  if (files.length) { event.preventDefault(); uploadFiles(files); }
});
let dragDepth = 0;
document.addEventListener('dragenter', event => {
  if (!event.dataTransfer.types.includes('Files')) return;
  event.preventDefault();
  dragDepth++;
  $('drop-overlay').hidden = false;
});
document.addEventListener('dragover', event => { if (event.dataTransfer.types.includes('Files')) event.preventDefault(); });
document.addEventListener('dragleave', event => {
  if (!event.dataTransfer.types.includes('Files')) return;
  dragDepth--;
  if (dragDepth <= 0) $('drop-overlay').hidden = true;
});
document.addEventListener('drop', event => {
  event.preventDefault();
  dragDepth = 0;
  $('drop-overlay').hidden = true;
  uploadFiles([...event.dataTransfer.files]);
});

async function initialize() {
  try {
    const [config, sessions] = await Promise.all([api('/config'), api('/sessions')]);
    state.models = config.models;
    state.sessions = sessions;
    renderModels(state.models, localStorage.getItem('pureagent-model'));
    renderHistory(sessions);
    const selected = new URLSearchParams(location.search).get('session');
    if (selected) await openSession(selected);
    else updateControls();
    const savedDrafts = sessionStorage.getItem('pureagent-update-drafts');
    if (savedDrafts) {
      sessionStorage.removeItem('pureagent-update-drafts');
      for (const [id, draft] of JSON.parse(savedDrafts)) drafts.set(id, draft);
      restoreDraft(state.session?.id);
    }
    schedulePoll();
  } catch (error) { showError('composer-error', `无法连接本地工作空间：${error.message}`); }
}

initialize();
initializeUpdates(() => {
  saveDraft();
  sessionStorage.setItem('pureagent-update-drafts', JSON.stringify([...drafts]));
});
