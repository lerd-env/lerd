import { describe, it, expect, beforeEach } from 'vitest';
import { tablist } from './tablist';

describe('tablist', () => {
  let node: HTMLDivElement;
  let clicked: string[];

  beforeEach(() => {
    document.body.innerHTML = `<div id="l"><button role="tab" id="a">a</button><button role="tab" id="b">b</button><button role="tab" id="c">c</button></div>`;
    node = document.getElementById('l') as HTMLDivElement;
    clicked = [];
    node.querySelectorAll('button').forEach((b) => b.addEventListener('click', () => clicked.push(b.id)));
    tablist(node);
  });

  const press = (id: string, key: string) =>
    document.getElementById(id)!.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));

  it('selects and focuses the next tab, wrapping at the end', () => {
    press('c', 'ArrowRight');
    expect(clicked).toEqual(['a']);
    expect(document.activeElement?.id).toBe('a');
  });

  it('goes back with ArrowLeft, wrapping at the start', () => {
    press('a', 'ArrowLeft');
    expect(clicked).toEqual(['c']);
  });

  it('jumps with Home and End', () => {
    press('b', 'End');
    press('b', 'Home');
    expect(clicked).toEqual(['c', 'a']);
  });

  it('leaves other keys alone', () => {
    press('b', 'Enter');
    expect(clicked).toEqual([]);
  });
});
