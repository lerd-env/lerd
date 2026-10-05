import { describe, it, expect, beforeEach } from 'vitest';
import { selectorFor, describeElement, notePlace } from './annotate';

beforeEach(() => {
  document.body.innerHTML = '';
});

describe('selectorFor', () => {
  it('uses a unique id', () => {
    document.body.innerHTML = '<main><button id="checkout">Pay</button></main>';
    expect(selectorFor(document.getElementById('checkout')!)).toBe('#checkout');
  });

  it('prefers a test attribute to a path', () => {
    document.body.innerHTML = '<ul><li><a data-testid="cart-link">Cart</a></li></ul>';
    const sel = selectorFor(document.querySelector('a')!);
    expect(sel).toBe('[data-testid="cart-link"]');
  });

  it('anchors a path at the nearest ancestor with an id and finds the element again', () => {
    document.body.innerHTML = '<section id="list"><div class="row"><span>a</span></div><div class="row"><span>b</span></div></section>';
    const target = document.querySelectorAll('span')[1];
    const sel = selectorFor(target);
    expect(sel.startsWith('#list')).toBe(true);
    expect(document.querySelectorAll(sel)).toHaveLength(1);
    expect(document.querySelector(sel)).toBe(target);
  });

  it('ignores an id that is not unique', () => {
    document.body.innerHTML = '<p id="x">one</p><p id="x">two</p>';
    const target = document.querySelectorAll('p')[1];
    expect(document.querySelector(selectorFor(target))).toBe(target);
  });
});

describe('describeElement', () => {
  it('gives the tag, the start of the text and where it sits on the page', () => {
    document.body.innerHTML = `<button id="b">  Pay   ${'now '.repeat(40)}</button>`;
    const d = describeElement(document.getElementById('b')!);
    expect(d.tag).toBe('button');
    expect(d.text.startsWith('Pay now')).toBe(true);
    expect(d.text.length).toBeLessThanOrEqual(120);
    expect(d.rect).toEqual({ x: 0, y: 0, w: 0, h: 0 });
  });
});

describe('notePlace', () => {
  it('opens beside the element and stays inside the viewport', () => {
    expect(notePlace({ left: 10, top: 20, bottom: 40, right: 60 }, 1000, 800, 320, 200)).toEqual({ left: 10, top: 48 });
    expect(notePlace({ left: 900, top: 700, bottom: 760, right: 980 }, 1000, 800, 320, 200)).toEqual({ left: 672, top: 492 });
  });
});

