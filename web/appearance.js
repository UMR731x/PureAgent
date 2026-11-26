// Apply the saved palette before styles and page content load, including on refresh.
(() => {
  const systemTheme = window.matchMedia('(prefers-color-scheme: dark)');
  const saved = localStorage.getItem('pureagent-appearance');
  const initial = ['light', 'dark', 'system'].includes(saved) ? saved : 'light';
  const root = document.documentElement;

  function apply(mode) {
    const dark = mode === 'dark' || (mode === 'system' && systemTheme.matches);
    root.dataset.appearance = mode;
    root.dataset.theme = dark ? 'dark' : 'light';
    document.querySelector('meta[name="theme-color"]').content = dark ? '#141b17' : '#f8f9f5';
  }

  apply(initial);
  systemTheme.addEventListener('change', () => {
    if (root.dataset.appearance === 'system') apply('system');
  });

  document.addEventListener('DOMContentLoaded', () => {
    const select = document.getElementById('appearance');
    select.value = root.dataset.appearance;
    select.dispatchEvent(new Event('change')); // Refresh the existing styled select button.
    select.addEventListener('change', () => {
      apply(select.value);
      localStorage.setItem('pureagent-appearance', select.value);
    });
  });
})();
