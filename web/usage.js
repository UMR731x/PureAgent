import { $, api, escapeHTML, showError } from './api.js';

const number = value => new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 4 }).format(value);
const time = value => new Date(value).toLocaleString('zh-CN', { hour12: false });

function renderReport(report) {
  const quotas = (report.quotas || []).map(quota => `<article class="usage-quota">
    <div><h3>${escapeHTML(quota.name)}</h3><span>AFP</span></div>
    <p><strong>${number(quota.remaining)}</strong><span>剩余</span></p>
    <div class="usage-track"><span style="width:${quota.quota > 0 ? Math.min(100, Math.max(0, quota.used / quota.quota * 100)) : 0}%"></span></div>
    <small>已用 ${number(quota.used)} / 总额 ${number(quota.quota)}</small>
    ${quota.reset_time ? `<small>重置：${escapeHTML(time(quota.reset_time))}</small>` : ''}
  </article>`).join('');
  const metrics = (report.metrics || []).map(metric => `<div class="usage-metric"><span>${escapeHTML(metric.name)}</span><strong>${number(metric.value)} <small>${escapeHTML(metric.unit)}</small></strong></div>`).join('');
  $('usage-results').innerHTML = `<p class="usage-scope">${escapeHTML(report.scope)}${report.plan ? ` · ${escapeHTML(report.plan)}` : ''}</p>
    ${quotas ? `<div class="usage-quotas">${quotas}</div>` : ''}
    ${report.remaining_note ? `<p class="usage-notice">${escapeHTML(report.remaining_note)}</p>` : ''}
    ${report.period ? `<div class="usage-period"><h3>当前模型 · 本月用量</h3><small>${escapeHTML(report.period)}</small></div>` : ''}
    ${metrics}
    ${(report.notices || []).map(note => `<p class="usage-notice">${escapeHTML(note)}</p>`).join('')}
    <small class="usage-checked">查询时间：${escapeHTML(time(report.checked_at))}</small>`;
  showError('usage-error', (report.errors || []).join('\n\n'));
  $('usage-console').href = report.console_url;
}

function credentialsForm(model) {
  const openai = model.provider === 'openai';
  const saved = model.usage || {};
  $('usage-ark-fields').hidden = openai;
  $('usage-openai-fields').hidden = !openai;
  $('usage-auth-note').textContent = openai
    ? '需要组织管理员创建的 Admin API Key。仅查询 OpenAI 官方组织用量，第三方中转服务的余额请在其控制台查看。'
    : '需要火山引擎 Access Key / Secret Key，并开通相应 AgentPlan 查询权限。企业版还需填写席位 Seat ID。';
  for (const key of ['access', 'secret', 'admin']) {
    const input = $(`usage-${key}-key`);
    input.value = '';
    input.placeholder = saved[`has_${key}_key`] ? '已保存，留空保留' : `填写 ${key === 'access' ? 'Access Key' : key === 'secret' ? 'Secret Key' : 'Admin API Key'}`;
  }
  $('usage-seat').value = saved.seat_id || '';
  $('usage-credentials').open = openai ? !saved.has_admin_key : !(saved.has_access_key && saved.has_secret_key);
  $('usage-console').href = openai ? 'https://platform.openai.com/settings/organization/billing/overview'
    : 'https://ark.volcengine.com/region:cn-beijing/subscription/agent-plan';
}

export function initializeUsage(selectedModel) {
  const dialog = $('usage-dialog');
  let model;
  let pending;

  async function query(save = false) {
    pending?.abort();
    const controller = new AbortController();
    pending = controller;
    $('usage-refresh').disabled = true;
    $('usage-save').disabled = true;
    showError('usage-error');
    $('usage-results').innerHTML = '<p class="usage-loading"><span class="spinner"></span>正在查询官方用量…</p>';
    const path = `/models/${encodeURIComponent(model.id)}/usage`;
    try {
      if (save) {
        const body = model.provider === 'openai' ? { admin_key: $('usage-admin-key').value, seat_id: model.usage?.seat_id || '' }
          : { access_key: $('usage-access-key').value, secret_key: $('usage-secret-key').value, seat_id: $('usage-seat').value };
        model.usage = await api(path, { method: 'PUT', body, signal: controller.signal });
        credentialsForm(model);
      }
      const report = await api(path, { signal: controller.signal });
      renderReport(report);
    } catch (error) {
      if (controller.signal.aborted) return;
      $('usage-results').replaceChildren();
      showError('usage-error', error.message);
    } finally {
      if (pending === controller) {
        $('usage-refresh').disabled = false;
        $('usage-save').disabled = false;
      }
    }
  }

  $('usage-open').onclick = () => {
    model = selectedModel();
    if (!model) return;
    $('usage-model-name').textContent = model.name;
    $('usage-model-id').textContent = model.model_name;
    credentialsForm(model);
    dialog.showModal();
    if (!$('usage-credentials').open) {
      query();
    } else {
      showError('usage-error');
      $('usage-results').innerHTML = `<p class="usage-notice">${model.provider === 'openai'
        ? '配置查询凭据后可查看当前模型的用量。OpenAI 官方 API 不提供余额查询，剩余额度请前往官方控制台查看。'
        : '配置查询凭据后，可查看当前模型的本月用量，以及 AgentPlan 套餐共享的已用和剩余 AFP。'}</p>`;
    }
  };
  $('usage-close').onclick = () => dialog.close();
  dialog.addEventListener('close', () => {
    pending?.abort();
    $('usage-form').reset();
  });
  dialog.addEventListener('click', event => { if (event.target === dialog) dialog.close(); });
  $('usage-refresh').onclick = () => query();
  $('usage-form').onsubmit = event => { event.preventDefault(); query(true); };
}
