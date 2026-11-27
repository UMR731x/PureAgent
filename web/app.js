import { $, api, toast, showError } from './api.js';
import { hydrateIcons } from './icons.js';
import { renderConversation, renderAttachments, renderModels, modelConfigCard } from './views.js';
import { createViewer, copyImage } from './viewer.js';
import { initializeControls, refreshControls } from './controls.js';
import { initializeUpdates } from './updates.js';
import { syncModelOptions, selectedImageSize, restoreImageSize } from './model-options.js';
import { createEditMarkup } from './markup.js';
import { initializeUsage } from './usage.js';
import { HistoryView } from './history.js';

hydrateIcons();
initializeControls();
$('local-address').textContent = location.host;

const state = { session: null, sessions: [], models: [], references: [], referenceTurnID: '', markupID: '', layerMode: false, busy: false };
const drafts = new Map();
let pollTimer;
let navigationVersion = 0;
const historyView = new HistoryView(state, { onOpen: openSession, onDelete: deleteHistory });

function isOpenAI() { return state.models.find(model => model.id === $('model').value)?.provider === 'openai'; }
function referenceLimit() { return isOpenAI() ? 16 : 10; }

function options() {
  return {
    count: state.layerMode ? 1 : Number($('count').value),
    resolution: $('resolution').value, ratio: $('ratio').value,
    background: $('background').value, format: $('format').value,
    watermark: !isOpenAI() && $('watermark').checked, layer_decomposition: state.layerMode,
    max_images: Number($('max-images').value), text_recognition: $('text-recognition').checked,
    ...(isOpenAI() ? { size: selectedImageSize(), quality: $('quality').value, complexity: $('complexity').value } : {}),
    ...(isOpenAI() && $('format').value === 'gif' ? { frame_count: Number($('frame-count').value), frame_duration_ms: Number($('frame-duration').value) } : {}),
  };
}

function applyOptions(value = {}) {
  syncModelOptions(isOpenAI(), state.layerMode);
  $('count').value = value.count || 1;
  $('resolution').value = value.resolution || 'auto';
  $('ratio').value = value.ratio || 'auto';
  $('format').value = value.format || 'png';
  syncModelOptions(isOpenAI(), state.layerMode);
  $('background').value = value.background || 'auto';
  $('watermark').checked = value.watermark || false;
  $('max-images').value = value.max_images || 30;
  $('text-recognition').checked = value.text_recognition ?? true;
  restoreImageSize(value.size);
  $('quality').value = value.quality || 'auto';
  $('complexity').value = value.complexity || '';
  $('frame-count').value = value.frame_count || 9;
  $('frame-duration').value = value.frame_duration_ms || 200;
  setLayerMode(value.layer_decomposition || false);
}

function setLayerMode(enabled) {
  enabled = enabled && !isOpenAI();
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
  syncModelOptions(isOpenAI(), state.layerMode);
  $('edit-markup').hidden = !state.markupID;
  const running = state.session?.turns.some(turn => turn.status === 'running') || false;
  $('generate-button').hidden = running;
  $('stop-button').hidden = !running;
  $('generate-button').disabled = state.busy;
  $('upload-button').disabled = state.busy;
  $('generate-button').querySelector('span').textContent = state.busy ? '保存中' : state.layerMode ? '拆分' : '生成';
  const configured = state.models.find(model => model.id === $('model').value)?.has_key;
  const previousOutputs = state.session?.turns.at(-1)?.outputs || [];
  const hasPreviousImage = previousOutputs.some(output => state.references.includes(output.asset_id));
  const previousGIF = isOpenAI() && previousOutputs.some(output => state.session.assets.find(asset => asset.id === output.asset_id)?.mime === 'image/gif');
  $('composer-hint').textContent = !configured ? '首次使用，请在右上角配置 API Key'
    : state.layerMode ? '选择 1 张 PNG / JPEG 原图 · 支持文字 OCR'
    : state.references.length > referenceLimit() ? `已加入 ${state.references.length} 张图，当前模型最多使用 ${referenceLimit()} 张，请移除不需要的参考图`
    : hasPreviousImage ? `${state.references.length} 张参考图 · 已带入上一轮图片，可直接输入修改要求`
    : state.references.length ? `${state.references.length} 张参考图 · ${isOpenAI() ? '图 1 可添加局部标记' : '点击缩略图可局部编辑'}`
    : previousGIF ? 'GIF 已保存；OpenAI 参考图需为 PNG / JPEG / WebP，可使用上一轮指令重新生成'
    : isOpenAI() ? 'PNG / JPEG / WebP · 最多 16 张参考图' : '支持拖入或粘贴参考图';
}

function saveDraft() {
  drafts.set(state.session?.id || 'new', {
    prompt: $('prompt').value, references: [...state.references], referenceTurnID: state.referenceTurnID,
    markupID: state.markupID, options: options(), model: $('model').value,
  });
}

function restoreDraft(id) {
  const draft = drafts.get(id || 'new');
  $('prompt').value = draft?.prompt || '';
  state.references = draft?.references || [];
  state.referenceTurnID = draft?.referenceTurnID || '';
  if (draft?.model && state.models.some(model => model.id === draft.model)) $('model').value = draft.model;
  carryPreviousImages();
  state.markupID = isOpenAI() ? draft?.markupID || '' : '';
  applyOptions(draft?.options);
  renderAttachments(state.session, state.references);
  showError('composer-error');
  resizePrompt();
}

function carryPreviousImages() {
  const previous = state.session?.turns.at(-1);
  if (!previous || previous.status === 'running' || state.referenceTurnID === previous.id) return false;
  state.referenceTurnID = previous.id;
  const images = previous.outputs.filter(output => !isOpenAI() || state.session.assets.find(asset => asset.id === output.asset_id)?.mime !== 'image/gif');
  state.references = [...new Set([...state.references, ...images.map(output => output.asset_id)])];
  return true;
}

async function refreshHistory() {
  const version = navigationVersion;
  const sessions = await api('/sessions');
  if (version !== navigationVersion) return;
  state.sessions = sessions;
  historyView.render();
}

async function deleteHistory(ids) {
  if (state.busy) throw new Error('图片正在保存，请稍后删除任务');
  state.busy = true;
  ++navigationVersion;
  clearTimeout(pollTimer);
  updateControls();
  try {
    const result = await api('/sessions', { method: 'DELETE', body: { ids } });
    const deleted = new Set(result.deleted);
    for (const id of deleted) drafts.delete(id);
    state.sessions = state.sessions.filter(session => !deleted.has(session.id));
    if (deleted.has(state.session?.id)) {
      state.session = null;
      history.replaceState(null, '', '/');
      restoreDraft(null);
      renderConversation(null, true);
    }
    historyView.render();
    return result;
  } finally {
    state.busy = false;
    updateControls();
    schedulePoll();
  }
}

function schedulePoll() {
  clearTimeout(pollTimer);
  const currentRunning = state.session?.turns.some(turn => turn.status === 'running');
  if (!currentRunning && !state.sessions.some(session => session.running)) return;
  pollTimer = setTimeout(async () => {
    const version = navigationVersion;
    try {
      const currentID = state.session?.id;
      await refreshHistory();
      if (version !== navigationVersion) return;
      if (currentID) {
        const session = await api(`/sessions/${currentID}`);
        if (version === navigationVersion) {
          state.session = session;
          if (carryPreviousImages()) renderAttachments(session, state.references);
          renderConversation(session);
          updateControls();
        }
      }
      if (version === navigationVersion) schedulePoll();
    } catch (error) {
      if (version === navigationVersion) toast(`暂时无法读取生成进度：${error.message}。刷新页面可重新查看。`, true);
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
    historyView.render();
    $('sidebar').classList.remove('open');
    $('prompt').focus();
  } catch (error) { if (version === navigationVersion) toast(error.message, true); }
  finally { if (version === navigationVersion) schedulePoll(); }
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
  if (state.references.length >= referenceLimit()) {
    toast(`当前模型每次最多使用 ${referenceLimit()} 张参考图，请先移除一张。`, true);
    return false;
  }
  if (isOpenAI() && !['image/png', 'image/jpeg', 'image/webp'].includes(asset.mime)) {
    toast('OpenAI 参考图需为 PNG、JPEG 或 WebP。', true);
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
  supportsLayers: () => !isOpenAI(),
  selectionNotice: asset => isOpenAI() && state.references.length && state.references[0] !== asset.id
    ? 'OpenAI 局部标记作用于图 1，请先将这张图片作为第一张参考图。' : '',
  async onSelection(asset, token, selection) {
    if (!editImage(asset)) return false;
    if (isOpenAI()) return addOpenAIMarkup(selection);
    const number = state.references.indexOf(asset.id) + 1;
    const input = $('prompt');
    const position = input.selectionStart;
    input.setRangeText(`图 ${number} ${token} `, position, input.selectionEnd, 'end');
    input.focus();
    resizePrompt();
    return true;
  },
});

async function uploadAsset(file) {
  const session = await ensureSession();
  const body = new FormData();
  body.append('file', file);
  const asset = await api(`/sessions/${session.id}/assets`, { method: 'POST', body });
  state.session.assets.push(asset);
  return asset;
}

async function addOpenAIMarkup(selection) {
  if (state.busy) return false;
  state.busy = true;
  updateControls();
  try {
    const file = await createEditMarkup($('viewer-image'), selection);
    const markup = await uploadAsset(file);
    state.markupID = markup.id;
    const input = $('prompt');
    const text = selection.mode === 'point' ? '修改图 1 中红圈标记的对象：' : '修改图 1 中红框标记的区域：';
    input.setRangeText(text, input.selectionStart, input.selectionEnd, 'end');
    resizePrompt();
    return true;
  } finally { state.busy = false; updateControls(); }
}

async function uploadFiles(files) {
  if (!files.length || state.busy) return;
  if (state.references.length + files.length > referenceLimit()) {
    showError('composer-error', `当前模型每次最多接受 ${referenceLimit()} 张参考图。`);
    return;
  }
  const maxBytes = (isOpenAI() ? 50 : 30) * 1024 * 1024;
  if (files.some(file => isOpenAI() ? file.size >= maxBytes : file.size > maxBytes)) {
    showError('composer-error', isOpenAI() ? 'OpenAI 单张参考图需小于 50 MiB。' : 'Seedream 单张参考图不能超过 30 MiB。');
    return;
  }
  state.busy = true;
  updateControls();
  showError('composer-error');
  try {
    for (const file of files) {
      const asset = await uploadAsset(file);
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
    ...(isOpenAI() && state.markupID ? { markup_id: state.markupID } : {}),
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
    state.markupID = '';
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
  initializeControls($('model-configs'));
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
    provider: card.querySelector('[name=provider]').value,
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
    if (!isOpenAI()) state.markupID = '';
    setLayerMode(state.layerMode);
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
    state.markupID = isOpenAI() ? turn.markup_id || '' : '';
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
$('conversation').addEventListener('click', handleImageAction);
$('attachments').addEventListener('click', event => {
  handleImageAction(event);
  const button = event.target.closest('[data-remove]');
  if (!button) return;
  const removedNumber = state.references.indexOf(button.dataset.remove) + 1;
  if (removedNumber === 1) state.markupID = '';
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
  if ($('background').value === 'transparent' && $('format').value === 'jpeg') $('format').value = 'png';
  refreshControls();
};
$('model').onchange = () => {
  localStorage.setItem('pureagent-model', $('model').value);
  if (!isOpenAI()) state.markupID = '';
  setLayerMode(state.layerMode);
};
$('image-size').onchange = updateControls;
$('format').onchange = updateControls;
$('remove-markup').onclick = () => { state.markupID = ''; updateControls(); };
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
  initializeControls(card);
  card.querySelector('input').focus();
};
$('model-configs').onclick = event => {
  const button = event.target.closest('.remove-model');
  if (button) { button.closest('.model-config').remove(); refreshControls(); }
};
$('menu-button').onclick = () => $('sidebar').classList.toggle('open');
document.addEventListener('click', event => {
  if (!event.target.closest('#sidebar, #menu-button, #history-delete-dialog')) $('sidebar').classList.remove('open');
});

document.addEventListener('keydown', event => {
  if (event.key === 'Escape' && $('settings-drawer').classList.contains('open')) closeSettings();
  if ((event.ctrlKey || event.metaKey) && event.key === 'Enter' && !document.querySelector('dialog[open]') && !$('settings-drawer').classList.contains('open')) {
    event.preventDefault();
    if (!$('generate-button').hidden) $('composer').requestSubmit();
  }
  if (event.key.toLowerCase() === 'n' && !event.ctrlKey && !event.metaKey && !event.altKey && !event.target.closest('input, textarea, select, [role=combobox], dialog, [contenteditable=true]') && !$('settings-drawer').classList.contains('open')) {
    event.preventDefault();
    openSession(null);
  }
});

document.addEventListener('paste', event => {
  if (event.target.closest('#settings-drawer, dialog')) return;
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
    historyView.render();
    const desktopDraft = JSON.parse(localStorage.getItem('pureagent-desktop-update') || 'null');
    const selected = desktopDraft ? desktopDraft.session : new URLSearchParams(location.search).get('session');
    if (selected) await openSession(selected);
    else updateControls();
    const savedDrafts = desktopDraft?.drafts || JSON.parse(sessionStorage.getItem('pureagent-update-drafts') || 'null');
    if (savedDrafts) {
      for (const [id, draft] of savedDrafts) drafts.set(id, draft);
      restoreDraft(state.session?.id);
      sessionStorage.removeItem('pureagent-update-drafts');
      localStorage.removeItem('pureagent-desktop-update');
    }
    schedulePoll();
  } catch (error) { showError('composer-error', `无法连接本地工作空间：${error.message}`); }
}

initialize();
initializeUsage(() => state.models.find(model => model.id === $('model').value));
initializeUpdates(() => {
  saveDraft();
  sessionStorage.setItem('pureagent-update-drafts', JSON.stringify([...drafts]));
});
window.addEventListener('pureagent-before-update', () => {
  saveDraft();
  localStorage.setItem('pureagent-desktop-update', JSON.stringify({ session: state.session?.id, drafts: [...drafts] }));
});
