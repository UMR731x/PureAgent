import { $, escapeHTML as esc, showError, toast } from './api.js';
import { icon } from './icons.js';

export class HistoryView {
  constructor(state, { onOpen, onDelete }) {
    this.state = state;
    this.onDelete = onDelete;
    this.selected = new Set();
    this.dialog = $('history-delete-dialog');
    this.selecting = false;
    this.pending = [];
    this.deleting = false;
    this.renderedHistory = '';
    this.bindEvents(onOpen);
  }

  updateSelection() {
    const eligible = new Set(this.state.sessions.filter(session => !session.running).map(session => session.id));
    for (const id of this.selected) {
      if (!eligible.has(id)) this.selected.delete(id);
    }
    $('history-select-all').checked = eligible.size > 0 && this.selected.size === eligible.size;
    $('history-select-all').indeterminate = this.selected.size > 0 && this.selected.size < eligible.size;
    $('history-select-all').disabled = !eligible.size;
    $('history-selected-count').textContent = `已选 ${this.selected.size} 项`;
    $('history-delete-selected').disabled = !this.selected.size;
    $('history-running-note').hidden = !this.selecting || !this.state.sessions.some(session => session.running);
    $('history').querySelectorAll('[data-select-session]').forEach(input => {
      input.checked = this.selected.has(input.dataset.selectSession);
      input.closest('.history-row').classList.toggle('selected', input.checked);
    });
  }

  render() {
    if (!this.state.sessions.length) this.selecting = false;
    $('session-count').textContent = this.state.sessions.length;
    $('history-manage').textContent = this.selecting ? '完成' : '管理';
    $('history-manage').disabled = !this.state.sessions.length;
    $('history-manage').setAttribute('aria-pressed', String(this.selecting));
    $('history-selection').hidden = !this.selecting;
    const snapshot = JSON.stringify([this.state.sessions, this.state.session?.id, this.selecting]);
    if (snapshot !== this.renderedHistory) {
      const focused = $('history').contains(document.activeElement) ? document.activeElement : null;
      let previousGroup = '';
      $('history').innerHTML = this.state.sessions.map(session => {
        const group = new Date(session.updated_at).toDateString() === new Date().toDateString() ? '今天' : '更早的创作';
        const heading = group !== previousGroup ? `<div class="history-group">${group}</div>` : '';
        previousGroup = group;
        return heading + historyRow(session, this.state.session?.id, this.selecting);
      }).join('') || `<div class="history-empty">${icon('images')}<div>这里会收好<br>你的每一次灵感</div></div>`;
      if (focused) {
        const attribute = ['data-session', 'data-select-session', 'data-delete-session'].find(name => focused.hasAttribute(name));
        if (attribute) $('history').querySelector(`[${attribute}="${CSS.escape(focused.getAttribute(attribute))}"]`)?.focus();
      }
      this.renderedHistory = snapshot;
    }
    this.updateSelection();
  }

  confirmDeletion(ids) {
    this.pending = this.state.sessions.filter(session => ids.includes(session.id));
    $('history-delete-title').textContent = `删除 ${this.pending.length} 个历史会话？`;
    $('history-delete-list').innerHTML = this.pending.map(session => `<li>${esc(session.title)}</li>`).join('');
    $('history-delete-confirm').textContent = `删除 ${this.pending.length} 个会话`;
    showError('history-delete-error');
    this.dialog.showModal();
    $('history-delete-cancel').focus();
  }

  async deletePending() {
    this.deleting = true;
    $('history-delete-confirm').disabled = true;
    $('history-delete-cancel').disabled = true;
    $('history-delete-confirm').textContent = '正在删除…';
    showError('history-delete-error');
    try {
      const result = await this.onDelete(this.pending.map(session => session.id));
      for (const id of result.deleted) this.selected.delete(id);
      if (result.failed.length) {
        const messages = result.failed.map(failure => {
          const title = this.pending.find(session => session.id === failure.id)?.title || failure.id;
          return `${title}：${failure.message}`;
        });
        this.pending = this.pending.filter(session => result.failed.some(failure => failure.id === session.id));
        $('history-delete-title').textContent = `${this.pending.length} 个会话未删除`;
        $('history-delete-list').innerHTML = this.pending.map(session => `<li>${esc(session.title)}</li>`).join('');
        showError('history-delete-error', [result.deleted.length ? `已删除 ${result.deleted.length} 个会话。` : '', ...messages].filter(Boolean).join('\n'));
      } else {
        this.dialog.close();
        toast(`已删除 ${result.deleted.length} 个会话及其图片`);
      }
      this.render();
    } catch (error) {
      showError('history-delete-error', error.message);
    } finally {
      this.deleting = false;
      $('history-delete-confirm').disabled = false;
      $('history-delete-cancel').disabled = false;
      $('history-delete-confirm').textContent = `删除 ${this.pending.length} 个会话`;
    }
  }

  bindEvents(onOpen) {
    $('history-manage').onclick = () => {
      this.selecting = !this.selecting;
      this.selected.clear();
      this.render();
    };
    $('history-select-all').onchange = event => {
      this.selected.clear();
      if (event.target.checked) this.state.sessions.filter(session => !session.running).forEach(session => this.selected.add(session.id));
      this.updateSelection();
    };
    $('history-delete-selected').onclick = () => this.confirmDeletion([...this.selected]);
    $('history').onclick = event => {
      const remove = event.target.closest('[data-delete-session]');
      if (remove) { this.confirmDeletion([remove.dataset.deleteSession]); return; }
      const open = event.target.closest('[data-session]');
      if (open) onOpen(open.dataset.session);
    };
    $('history').onchange = event => {
      const input = event.target.closest('[data-select-session]');
      if (!input) return;
      if (input.checked) this.selected.add(input.dataset.selectSession);
      else this.selected.delete(input.dataset.selectSession);
      this.updateSelection();
    };
    $('history-delete-cancel').onclick = () => this.dialog.close();
    $('history-delete-confirm').onclick = () => this.deletePending();
    this.dialog.addEventListener('cancel', event => { if (this.deleting) event.preventDefault(); });
    this.dialog.addEventListener('close', () => {
      if (!$('history').contains(document.activeElement)) ($('history-manage').disabled ? $('new-task') : $('history-manage')).focus();
    });
  }
}

function historyRow(session, activeID, selecting) {
  const id = esc(session.id);
  const title = esc(session.title);
  const progress = session.running ? '<i class="running-dot" aria-label="正在生成"></i>' : '';
  const content = selecting
    ? `<label class="history-item history-choice" title="${session.running ? '正在生成，请先停止后再删除' : title}"><input type="checkbox" data-select-session="${id}" aria-label="选择 ${title}" ${session.running ? 'disabled' : ''}><span>${title}</span>${progress}</label>`
    : `<button class="history-item" data-session="${id}" title="${title}">${icon('image')}<span>${title}</span>${progress}</button><button class="icon-button history-remove" data-delete-session="${id}" aria-label="删除 ${title}" title="${session.running ? '正在生成，请先停止后再删除' : '删除会话'}" ${session.running ? 'disabled' : ''}>${icon('trash')}</button>`;
  return `<div class="history-row ${session.id === activeID ? 'active' : ''}">${content}</div>`;
}
