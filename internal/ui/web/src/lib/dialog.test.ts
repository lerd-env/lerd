import { describe, it, expect, beforeEach } from 'vitest';
import { dialog } from './dialog';

function tab(shift = false) {
  const e = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: shift, bubbles: true, cancelable: true });
  document.activeElement!.dispatchEvent(e);
  return e;
}

describe('dialog', () => {
  let trigger: HTMLButtonElement;
  let panel: HTMLDivElement;

  beforeEach(() => {
    document.body.innerHTML = `
      <button id="trigger">open</button>
      <div id="panel"><button id="a">a</button><input id="b" /><button id="c" disabled>c</button></div>`;
    trigger = document.getElementById('trigger') as HTMLButtonElement;
    panel = document.getElementById('panel') as HTMLDivElement;
    trigger.focus();
  });

  it('moves focus to the first field in the panel', () => {
    dialog(panel);
    expect(document.activeElement?.id).toBe('b');
  });

  it('falls back to the first control when the panel has no field', () => {
    document.getElementById('b')!.remove();
    dialog(panel);
    expect(document.activeElement?.id).toBe('a');
  });

  it('leaves focus alone when a field inside already took it', () => {
    (document.getElementById('b') as HTMLInputElement).focus();
    dialog(panel);
    expect(document.activeElement?.id).toBe('b');
  });

  it('wraps Tab from the last control to the first, skipping disabled ones', () => {
    dialog(panel);
    (document.getElementById('b') as HTMLInputElement).focus();
    expect(tab().defaultPrevented).toBe(true);
    expect(document.activeElement?.id).toBe('a');
  });

  it('wraps Shift+Tab from the first control to the last', () => {
    dialog(panel);
    (document.getElementById('a') as HTMLButtonElement).focus();
    expect(tab(true).defaultPrevented).toBe(true);
    expect(document.activeElement?.id).toBe('b');
  });

  it('lets Tab move normally between controls in the middle', () => {
    dialog(panel);
    (document.getElementById('a') as HTMLButtonElement).focus();
    expect(tab().defaultPrevented).toBe(false);
  });

  it('gives focus back to the trigger on close', () => {
    const action = dialog(panel);
    action.destroy();
    expect(document.activeElement).toBe(trigger);
  });
});
