import { $, assetURL, escapeHTML as esc, fileSize, toast } from './api.js';
import { icon } from './icons.js';

export async function copyImage(sessionID, asset) {
  if (!navigator.clipboard?.write || !window.ClipboardItem) {
    throw new Error('当前浏览器不支持复制图片，请使用下载原图。');
  }
  const png = (async () => {
    const response = await fetch(assetURL(sessionID, asset.id));
    if (!response.ok) throw new Error('无法读取本地原图');
    const blob = await response.blob();
    if (blob.type === 'image/png') return blob;
    const bitmap = await createImageBitmap(blob);
    const canvas = document.createElement('canvas');
    canvas.width = bitmap.width;
    canvas.height = bitmap.height;
    canvas.getContext('2d').drawImage(bitmap, 0, 0);
    bitmap.close();
    return new Promise((resolve, reject) => canvas.toBlob(result => result ? resolve(result) : reject(new Error('无法复制此图片格式')), 'image/png'));
  })();
  await navigator.clipboard.write([new ClipboardItem({ 'image/png': png })]);
  toast(asset.mime === 'image/png' ? '图片已复制' : '已按原始尺寸复制为 PNG');
}

export function createViewer(callbacks) {
  let session;
  let asset;
  let selectionMode = 'bbox';
  let selection;
  let pointerStart;

  function metadata(output) {
    const panel = $('image-metadata');
    panel.hidden = !output || (!output.name && !output.bounding_box && !output.text_recognition);
    if (panel.hidden) return;
    const recognition = output.text_recognition;
    panel.innerHTML = `<h3>${esc(output.name || '图层信息')}</h3>
      ${output.description ? `<p>${esc(output.description)}</p>` : ''}
      ${output.z_index != null ? `<p>图层顺序 ${esc(output.z_index)}</p>` : ''}
      ${recognition?.text ? `<h3>识别的文字</h3><div class="ocr-text">${esc(recognition.text)}</div><button class="subtle-button" id="copy-ocr">${icon('copy')}复制文字</button>` : ''}
      <details><summary>位置与文字样式</summary><pre>${esc(JSON.stringify({ bounding_box: output.bounding_box, text_recognition: recognition }, null, 2))}</pre></details>`;
    if ($('copy-ocr')) $('copy-ocr').onclick = async () => {
      try {
        await navigator.clipboard.writeText(recognition.text);
        toast('文字已复制');
      } catch (error) { toast(error.message, true); }
    };
  }

  function resetSelection() {
    selection = null;
    pointerStart = null;
    $('selection-overlay').replaceChildren();
    $('insert-selection').disabled = true;
  }

  function open(currentSession, selectedAsset, localEdit = false) {
    session = currentSession;
    asset = selectedAsset;
    $('viewer-image').src = assetURL(session.id, asset.id);
    $('viewer-title').textContent = asset.name;
    $('viewer-dimensions').textContent = `${asset.width ? `${asset.width} × ${asset.height} · ` : ''}${fileSize(asset.bytes)}`;
    $('viewer-download').href = assetURL(session.id, asset.id, true);
    $('viewer-download').download = asset.name;
    $('viewer-stage').classList.remove('original');
    $('selection-toolbar').hidden = true;
    $('image-surface').classList.remove('selecting');
    resetSelection();
    metadata(session.turns.flatMap(turn => turn.outputs).find(output => output.asset_id === asset.id));
    if (!$('image-dialog').open) $('image-dialog').showModal();
    if (localEdit) startSelection();
  }

  function close() { $('image-dialog').close(); }

  function startSelection() {
    $('selection-toolbar').hidden = false;
    $('image-surface').classList.add('selecting');
    resetSelection();
  }

  function coordinates(event) {
    const bounds = $('viewer-image').getBoundingClientRect();
    return {
      x: Math.min(999, Math.max(0, Math.round((event.clientX - bounds.left) / bounds.width * 1000))),
      y: Math.min(999, Math.max(0, Math.round((event.clientY - bounds.top) / bounds.height * 1000))),
    };
  }

  function drawSelection() {
    const overlay = $('selection-overlay');
    overlay.replaceChildren();
    if (!selection) return;
    const marker = document.createElement('div');
    marker.className = selection.mode === 'point' ? 'selection-point' : 'selection-box';
    marker.style.left = `${selection.x1 / 10}%`;
    marker.style.top = `${selection.y1 / 10}%`;
    if (selection.mode === 'bbox') {
      marker.style.width = `${(selection.x2 - selection.x1) / 10}%`;
      marker.style.height = `${(selection.y2 - selection.y1) / 10}%`;
    }
    overlay.append(marker);
  }

  $('image-surface').addEventListener('pointerdown', event => {
    if ($('selection-toolbar').hidden) return;
    event.preventDefault();
    $('image-surface').setPointerCapture(event.pointerId);
    pointerStart = coordinates(event);
    selection = { mode: selectionMode, x1: pointerStart.x, y1: pointerStart.y, x2: pointerStart.x, y2: pointerStart.y };
    drawSelection();
  });

  $('image-surface').addEventListener('pointermove', event => {
    if (!pointerStart || selectionMode !== 'bbox') return;
    const end = coordinates(event);
    selection = {
      mode: 'bbox', x1: Math.min(pointerStart.x, end.x), y1: Math.min(pointerStart.y, end.y),
      x2: Math.max(pointerStart.x, end.x), y2: Math.max(pointerStart.y, end.y),
    };
    drawSelection();
  });

  $('image-surface').addEventListener('pointerup', event => {
    if (!pointerStart) return;
    pointerStart = null;
    $('image-surface').releasePointerCapture(event.pointerId);
    $('insert-selection').disabled = selection.mode === 'bbox' && (selection.x1 === selection.x2 || selection.y1 === selection.y2);
  });

  for (const [id, mode] of [['select-box', 'bbox'], ['select-point', 'point']]) {
    $(id).onclick = () => {
      selectionMode = mode;
      $('select-box').classList.toggle('active', mode === 'bbox');
      $('select-point').classList.toggle('active', mode === 'point');
      $('selection-hint').textContent = mode === 'bbox' ? '在图片上拖动选择编辑区域' : '点击想修改的对象';
      resetSelection();
    };
  }

  $('insert-selection').onclick = () => {
    const token = selection.mode === 'point'
      ? `<point>${selection.x1} ${selection.y1}</point>`
      : `<bbox>${selection.x1} ${selection.y1} ${selection.x2} ${selection.y2}</bbox>`;
    if (callbacks.onSelection(asset, token)) close();
  };
  $('viewer-close').onclick = close;
  $('viewer-edit').onclick = () => { if (callbacks.onEdit(asset)) close(); };
  $('viewer-select').onclick = startSelection;
  $('viewer-layers').onclick = () => { callbacks.onLayers(asset); close(); };
  $('viewer-zoom').onclick = () => $('viewer-stage').classList.toggle('original');
  $('viewer-copy').onclick = () => copyImage(session.id, asset).catch(error => toast(error.message, true));
  $('image-dialog').addEventListener('click', event => { if (event.target === $('image-dialog')) close(); });
  $('viewer-image').addEventListener('error', () => {
    $('viewer-dimensions').textContent = '浏览器无法预览此格式，可以下载原图或直接用作参考图。';
  });
  return { open };
}
