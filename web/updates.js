import { $, api } from './api.js';

const activePhases = new Set(['checking', 'downloading', 'waiting', 'restarting']);
const labels = {
  checking: '正在检查更新', downloading: '正在下载新版本', waiting: '等待任务完成',
  restarting: '正在重启', latest: '已是最新版本', error: '重新检查更新', idle: '检查并更新',
};

export function initializeUpdates(beforeReload) {
  let current;
  let timer;
  let updating = false;

  function render(status) {
    if (current && status.current !== current) {
      beforeReload();
      location.reload();
      return;
    }
    current = status.current;
    updating = activePhases.has(status.phase);
    $('app-version').textContent = status.current === 'dev' ? '源码开发版' : `PureAgent ${status.current}`;
    $('update-label').textContent = labels[status.phase] || '检查并更新';
    $('update-button').disabled = updating;
    $('update-status').textContent = status.message || '';
    $('update-status').hidden = !status.message;
    $('update-status').classList.toggle('error', status.phase === 'error');
    $('update-release').hidden = !status.release_url;
    if (status.release_url) $('update-release').href = status.release_url;
    clearTimeout(timer);
    if (updating) timer = setTimeout(poll, 1000);
  }

  async function poll() {
    try {
      render(await api('/update'));
    } catch (error) {
      $('update-status').hidden = false;
      $('update-status').textContent = updating
        ? '等待客户端恢复连接… 页面会自动恢复；若程序未重新启动，可从安装目录手动启动。'
        : `无法读取客户端版本：${error.message}`;
      if (updating) timer = setTimeout(poll, 1000);
    }
  }

  $('update-button').onclick = async () => {
    $('update-button').disabled = true;
    $('update-label').textContent = '正在检查更新';
    try {
      render(await api('/update', { method: 'POST' }));
    } catch (error) {
      $('update-button').disabled = false;
      $('update-label').textContent = '重新检查更新';
      $('update-status').hidden = false;
      $('update-status').classList.add('error');
      $('update-status').textContent = error.message;
    }
  };
  poll();
}
