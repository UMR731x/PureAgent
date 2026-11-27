import { $, escapeHTML as esc, assetURL } from './api.js';
import { icon } from './icons.js';

function imageCard(session, output) {
  const asset = session.assets.find(item => item.id === output.asset_id);
  if (!asset) return '';
  const label = output.name || output.size || (asset.width ? `${asset.width} × ${asset.height}` : '原图');
  const copyLabel = asset.mime === 'image/gif' ? '复制首帧' : '复制图片';
  return `<figure class="image-card">
    <button class="image-card-preview" data-view="${esc(asset.id)}" aria-label="查看 ${esc(label)}"><img src="${assetURL(session.id, asset.id)}" alt="${esc(output.description || output.name || '模型生成图像')}" loading="lazy"><span class="image-card-hover">${icon('zoom')}</span></button>
    <figcaption class="image-card-footer"><span class="image-card-name" title="${esc(label)}">${esc(label)}</span><div class="image-actions">
      <button class="icon-button" data-edit="${esc(asset.id)}" title="编辑图片" aria-label="编辑图片">${icon('pen')}</button>
      <button class="icon-button" data-copy="${esc(asset.id)}" title="${copyLabel}" aria-label="${copyLabel}">${icon('copy')}</button>
      <a class="icon-button" href="${assetURL(session.id, asset.id, true)}" download title="下载原图" aria-label="下载原图">${icon('download')}</a>
    </div></figcaption>
  </figure>`;
}

function errorCard(turn) {
  if (!turn.error) return '';
  const error = turn.error;
  const title = { cancelled: '生成已停止', interrupted: '生成被中断', failed: '这次生成没有完成' }[turn.status] || '生成信息';
  return `<div class="turn-error"><strong>${icon('alert')}${title}</strong><p>${esc(error.message)}</p>
    ${error.code ? `<div>错误码：${esc(error.code)}</div>` : ''}
    ${error.http_status ? `<div>HTTP ${esc(error.http_status)}</div>` : ''}
    ${error.request_id ? `<div>Request ID：${esc(error.request_id)}</div>` : ''}
    ${error.details ? `<details><summary>接口返回详情</summary><pre>${esc(error.details)}</pre></details>` : ''}
  </div>`;
}

function turnCard(session, turn) {
  const options = turn.options;
  const references = turn.reference_ids || [];
  const time = new Date(turn.created_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
  const sizeLabel = options.size ? (options.size === 'auto' ? '自动尺寸' : options.size.replace('x', ' × '))
    : options.resolution === 'auto' ? '自动分辨率' : options.resolution;
  const tags = [options.layer_decomposition ? '图层拆分' : references.length ? '图像编辑' : '文字生图', sizeLabel];
  if (!options.layer_decomposition && !options.size) tags.push(options.ratio === 'auto' ? '自动比例' : options.ratio);
  if (turn.markup_id) tags.push('局部标记');
  if (options.format === 'gif') tags.push('GIF 动图');
  if (options.background === 'transparent' && !options.layer_decomposition) tags.push('透明背景');
  const referenceHTML = references.map((id, index) => `<button data-view="${esc(id)}" aria-label="查看参考图 ${index + 1}"><img src="${assetURL(session.id, id)}" alt="参考图 ${index + 1}"><span>图 ${index + 1}</span></button>`).join('');
  const progress = turn.status === 'running' ? `<div class="generation-progress"><span class="spinner"></span><div>${options.layer_decomposition ? '正在拆解图层' : '正在把灵感变成画面'}<small>${options.layer_decomposition ? '图层完成后会逐张保存' : `已完成 ${turn.completed} / ${options.count} 张`} · 可以切换任务，结果会留在这里</small></div></div>` : '';
  const finish = turn.status === 'completed' ? `${icon('check')} 已保存 ${turn.outputs.length} 张原图` : '';
  const details = { model: turn.model_name, options, usage: turn.usage, request_ids: turn.request_ids };
  return `<article class="turn" data-turn="${esc(turn.id)}">
    <div class="turn-heading"><span class="turn-badge">${icon('sparkles')}</span><strong>${esc(turn.model_name)}</strong><time>${time}</time></div>
    <div class="turn-prompt">${esc(turn.prompt || '自动识别并拆分图层')}</div>
    ${referenceHTML ? `<div class="turn-references">${referenceHTML}</div>` : ''}
    <div class="turn-tags">${tags.map(tag => `<span class="tag">${esc(tag)}</span>`).join('')}</div>
    <div class="output-grid ${turn.outputs.length === 1 ? 'single' : ''}">${turn.outputs.map(output => imageCard(session, output)).join('')}</div>
    ${progress}${errorCard(turn)}
    <div class="turn-footer"><span>${finish}</span><button class="subtle-button" data-reuse="${esc(turn.id)}" title="将本轮提示词和选项填回输入框">${icon('pen')}使用此指令</button></div>
    <details class="turn-details"><summary>生成参数与用量</summary><pre>${esc(JSON.stringify(details, null, 2))}</pre></details>
  </article>`;
}

let renderedTurns = '';
export function renderConversation(session, force = false) {
  const turns = session?.turns || [];
  const serialized = JSON.stringify(turns);
  $('studio').classList.toggle('has-turns', turns.length > 0);
  if (serialized === renderedTurns && !force) return;
  const container = $('conversation');
  const nearBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 100;
  const opened = Array.from(container.querySelectorAll('details[open]')).map(element => element.closest('.turn').dataset.turn);
  renderedTurns = serialized;
  container.innerHTML = turns.map(turn => turnCard(session, turn)).join('');
  for (const element of container.querySelectorAll('.turn-details')) {
    if (opened.includes(element.closest('.turn').dataset.turn)) element.open = true;
  }
  if (nearBottom || force) container.scrollTop = container.scrollHeight;
}

export function renderAttachments(session, ids) {
  $('attachments').hidden = !ids.length;
  $('attachments').innerHTML = ids.map((id, index) => `<div class="attachment">
    <button type="button" class="attachment-preview" data-view="${esc(id)}" title="查看或局部编辑参考图"><img src="${assetURL(session.id, id)}" alt="参考图 ${index + 1}"></button>
    <span class="attachment-label">图 ${index + 1}</span><button type="button" class="attachment-remove" data-remove="${esc(id)}" aria-label="移除参考图 ${index + 1}">${icon('x')}</button>
  </div>`).join('');
}

export function renderModels(models, selectedID) {
  $('model').innerHTML = models.length ? models.map(model => `<option value="${esc(model.id)}">${esc(model.name)}</option>`).join('') : '<option value="">请先添加模型</option>';
  if (models.some(model => model.id === selectedID)) $('model').value = selectedID;
  $('config-dot').classList.toggle('ready', models.some(model => model.has_key));
}

export function modelConfigCard(model = {}) {
  const card = document.createElement('section');
  card.className = 'model-config';
  card.dataset.id = model.id || '';
  card.innerHTML = `<div class="model-config-heading"><span>${icon('sparkles')}模型连接</span><button type="button" class="icon-button remove-model" title="移除此配置" aria-label="移除此模型配置">${icon('trash')}</button></div>
    <label>显示名称<input name="name" value="${esc(model.name || '')}" placeholder="例如：Seedream 5.5"></label>
    <label>接口类型<select name="provider" aria-label="接口类型"><option value="seedream">Seedream / AgentPlan</option><option value="openai">OpenAI Images · GPT Image 2.6</option></select></label>
    <label>MODEL_NAME<input name="model_name" value="${esc(model.model_name || '')}" placeholder="doubao-seedream-5.5" required spellcheck="false"></label>
    <label>API KEY<input name="api_key" type="password" autocomplete="new-password" placeholder="${model.has_key ? '已保存 · 留空保持不变' : '输入你的 API Key'}" spellcheck="false"></label>
    <label>接口 URL<input name="url" type="url" value="${esc(model.url || 'https://ark.cn-beijing.volces.com/api/plan/v3/images/generations')}" placeholder="完整的图片生成接口地址" required spellcheck="false"></label>
    <p class="model-protocol-note"></p>`;
  const provider = card.querySelector('[name=provider]');
  provider.value = model.provider || 'seedream';
  const describe = () => {
    card.querySelector('.model-protocol-note').textContent = provider.value === 'openai'
      ? '模型：gpt-image-2.6-flare 或 gpt-image-2.6-sunburst。URL 可填写 /v1 基础地址或图片接口完整地址。'
      : '使用 Seedream 图片生成接口，支持图层拆分与文字识别。';
  };
  provider.onchange = () => {
    const openAI = provider.value === 'openai';
    const name = card.querySelector('[name=model_name]');
    const url = card.querySelector('[name=url]');
    if (!name.value || /^(doubao-seedream-5\.5|gpt-image-2\.6-(flare|sunburst))$/.test(name.value)) {
      name.value = openAI ? 'gpt-image-2.6-flare' : 'doubao-seedream-5.5';
    }
    if (!url.value || url.value === 'https://api.openai.com/v1' || url.value === 'https://ark.cn-beijing.volces.com/api/plan/v3/images/generations') {
      url.value = openAI ? 'https://api.openai.com/v1' : 'https://ark.cn-beijing.volces.com/api/plan/v3/images/generations';
    }
    describe();
    if (model.has_key) {
      card.querySelector('[name=api_key]').placeholder = (model.provider || 'seedream') !== provider.value
        ? '接口类型已变更，请填写此接口的 API Key' : '已保存 · 留空保持不变';
    }
  };
  describe();
  return card;
}
