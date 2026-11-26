import { icon } from './icons.js';

let selects = [];
let numbers = [];
let opened;
let initialized = false;
let nextID = 0;

function closeMenu() {
  if (!opened) return;
  opened.menu.hidden = true;
  opened.button.setAttribute('aria-expanded', 'false');
  opened.button.removeAttribute('aria-activedescendant');
  opened = null;
}

function highlight(control, index) {
  control.active = index;
  [...control.menu.children].forEach((option, i) => option.classList.toggle('active', i === index));
  const option = control.menu.children[index];
  if (option) {
    control.button.setAttribute('aria-activedescendant', option.id);
    option.scrollIntoView({ block: 'nearest' });
  }
}

function choose(control, index) {
  control.select.selectedIndex = index;
  control.select.dispatchEvent(new Event('change', { bubbles: true }));
  refreshControls();
  closeMenu();
  control.button.focus();
}

function openMenu(control) {
  closeMenu();
  opened = control;
  const { select, button, menu } = control;
  menu.replaceChildren(...[...select.options].map((option, index) => {
    const item = document.createElement('div');
    item.id = `${select.id}-option-${index}`;
    item.className = 'select-option';
    item.setAttribute('role', 'option');
    item.setAttribute('aria-selected', String(index === select.selectedIndex));
    const label = document.createElement('span');
    label.textContent = option.text;
    item.append(label);
    item.insertAdjacentHTML('beforeend', icon('check'));
    item.onpointermove = () => highlight(control, index);
    item.onmousedown = event => event.preventDefault();
    item.onclick = () => choose(control, index);
    return item;
  }));
  menu.hidden = false;
  button.setAttribute('aria-expanded', 'true');
  const rect = button.getBoundingClientRect();
  menu.style.width = `${Math.min(Math.max(rect.width, 190), innerWidth - 24)}px`;
  menu.style.maxHeight = `${Math.min(280, Math.max(rect.top, innerHeight - rect.bottom) - 20)}px`;
  menu.style.left = `${Math.max(12, Math.min(rect.left, innerWidth - menu.offsetWidth - 12))}px`;
  menu.style.top = `${rect.bottom + menu.offsetHeight + 8 > innerHeight ? rect.top - menu.offsetHeight - 8 : rect.bottom + 8}px`;
  highlight(control, Math.max(0, select.selectedIndex));
}

function selectKey(control, event) {
  const { key } = event;
  if (key === 'Tab' || key === 'Escape') {
    if (opened === control) {
      closeMenu();
      if (key === 'Escape') { event.preventDefault(); event.stopPropagation(); }
    }
    return;
  }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End', 'Enter', ' '].includes(key) && key.length !== 1) return;
  if (event.ctrlKey || event.metaKey || event.altKey) return;
  event.preventDefault();
  event.stopPropagation();
  const wasOpen = opened === control;
  if (!wasOpen) openMenu(control);
  const last = control.select.options.length - 1;
  if (key === 'Home') highlight(control, 0);
  else if (key === 'End') highlight(control, last);
  else if (key === 'ArrowDown' && wasOpen) highlight(control, Math.min(last, control.active + 1));
  else if (key === 'ArrowUp' && wasOpen) highlight(control, Math.max(0, control.active - 1));
  else if ((key === 'Enter' || key === ' ') && wasOpen) choose(control, control.active);
  else if (key.length === 1 && key !== ' ') {
    const options = [...control.select.options];
    const next = options.findIndex((option, index) => index > control.active && option.text.toLowerCase().startsWith(key.toLowerCase()));
    const index = next < 0 ? options.findIndex(option => option.text.toLowerCase().startsWith(key.toLowerCase())) : next;
    if (index >= 0) highlight(control, index);
  }
}

function enhanceSelect(select) {
  if (!select.id) select.id = `select-${++nextID}`;
  const label = select.getAttribute('aria-label') || select.closest('label').firstChild.textContent.trim();
  const button = document.createElement('button');
  button.type = 'button';
  button.id = `${select.id}-control`;
  button.className = 'select-trigger';
  button.setAttribute('role', 'combobox');
  button.setAttribute('aria-haspopup', 'listbox');
  button.setAttribute('aria-expanded', 'false');
  button.setAttribute('aria-controls', `${select.id}-listbox`);
  button.innerHTML = `<span></span>${icon('chevron')}`;
  const menu = document.createElement('div');
  menu.id = `${select.id}-listbox`;
  menu.className = 'select-menu';
  menu.setAttribute('role', 'listbox');
  menu.setAttribute('aria-label', label);
  menu.hidden = true;
  document.body.append(menu);
  select.hidden = true;
  select.after(button);
  const control = { select, button, menu, label, active: 0 };
  selects.push(control);
  button.onclick = () => opened === control ? closeMenu() : openMenu(control);
  button.onkeydown = event => selectKey(control, event);
  select.addEventListener('change', refreshControls);
}

function enhanceNumber(input) {
  const wrapper = document.createElement('span');
  wrapper.className = 'number-control';
  input.before(wrapper);
  const label = input.getAttribute('aria-label') || '最多输出图像数量';
  input.setAttribute('aria-label', label);
  const buttons = [-1, 1].map(direction => {
    const button = document.createElement('button');
    button.type = 'button';
    button.textContent = direction < 0 ? '−' : '+';
    button.setAttribute('aria-label', `${direction < 0 ? '减少' : '增加'}${label}`);
    button.onclick = () => {
      direction < 0 ? input.stepDown() : input.stepUp();
      input.dispatchEvent(new Event('input', { bubbles: true }));
      input.dispatchEvent(new Event('change', { bubbles: true }));
      refreshControls();
    };
    return button;
  });
  wrapper.append(buttons[0], input, buttons[1]);
  numbers.push({ input, buttons });
  input.addEventListener('input', refreshControls);
}

export function refreshControls() {
  selects = selects.filter(control => {
    if (control.select.isConnected) return true;
    if (opened === control) closeMenu();
    control.menu.remove();
    return false;
  });
  numbers = numbers.filter(control => control.input.isConnected);
  for (const { select, button, label } of selects) {
    const text = select.selectedOptions[0]?.text || '';
    button.querySelector('span').textContent = text;
    button.setAttribute('aria-label', `${label}：${text}`);
    button.disabled = select.disabled;
  }
  for (const { input, buttons } of numbers) {
    buttons[0].disabled = input.disabled || (input.min !== '' && input.value !== '' && Number(input.value) <= Number(input.min));
    buttons[1].disabled = input.disabled || (input.max !== '' && input.value !== '' && Number(input.value) >= Number(input.max));
  }
  if (opened?.select.disabled) closeMenu();
}

export function initializeControls(root = document) {
  root.querySelectorAll('select').forEach(select => {
    if (!selects.some(control => control.select === select)) enhanceSelect(select);
  });
  root.querySelectorAll('input[type=number]').forEach(input => {
    if (!numbers.some(control => control.input === input)) enhanceNumber(input);
  });
  refreshControls();
  if (initialized) return;
  initialized = true;
  document.addEventListener('pointerdown', event => {
    if (opened && !opened.button.contains(event.target) && !opened.menu.contains(event.target)) closeMenu();
  });
  document.addEventListener('focusin', event => {
    if (opened && event.target !== opened.button) closeMenu();
  });
  document.addEventListener('scroll', event => {
    if (opened && !opened.menu.contains(event.target)) closeMenu();
  }, true);
  window.addEventListener('resize', closeMenu);
}
