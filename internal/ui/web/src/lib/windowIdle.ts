// Marks the root with data-window-idle while the window is blurred or hidden,
// so app.css can pause the infinite pulse animations. A visible but unfocused
// window is not throttled by the browser and would repaint them at full rate.
export function trackWindowIdle(): () => void {
  const root = document.documentElement;
  const set = (idle: boolean) => {
    if (idle) root.dataset.windowIdle = '';
    else delete root.dataset.windowIdle;
  };
  // The events carry the state themselves: hasFocus() can still report the old
  // value inside a focus or blur handler.
  const onFocus = () => set(document.hidden);
  const onBlur = () => set(true);
  const onVisibility = () => set(document.hidden || !document.hasFocus());
  window.addEventListener('focus', onFocus);
  window.addEventListener('blur', onBlur);
  document.addEventListener('visibilitychange', onVisibility);
  onVisibility();
  return () => {
    window.removeEventListener('focus', onFocus);
    window.removeEventListener('blur', onBlur);
    document.removeEventListener('visibilitychange', onVisibility);
  };
}
