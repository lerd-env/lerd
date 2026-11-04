import { describe, it, expect } from 'vitest';
import { peelBlockMarker, parseBlock, fontSize, FONT_MIN, FONT_MAX, splitRatio, SPLIT_MIN, SPLIT_MAX } from '$lib/tinker';

const US = String.fromCharCode(0x1f);
const STX = String.fromCharCode(0x02);

describe('peelBlockMarker', () => {
  it('extracts the line and strips the marker', () => {
    expect(peelBlockMarker(`9${US}hello`)).toEqual({ line: 9, rest: 'hello' });
    expect(peelBlockMarker(`12${US}`)).toEqual({ line: 12, rest: '' });
  });

  it('leaves a markerless chunk untouched', () => {
    expect(peelBlockMarker('hello')).toEqual({ rest: 'hello' });
  });

  it('does not treat plain numeric output as a marker', () => {
    expect(peelBlockMarker('42')).toEqual({ rest: '42' });
    expect(peelBlockMarker('42\n')).toEqual({ rest: '42\n' });
  });
});

describe('parseBlock', () => {
  it('classifies a statement-output block', () => {
    expect(parseBlock(`2${US}hello`)).toEqual({ line: 2, kind: 'output', body: 'hello' });
  });

  it('classifies a query block and strips the STX marker', () => {
    expect(parseBlock(`5${US}${STX}select * from users`)).toEqual({
      line: 5,
      kind: 'query',
      body: 'select * from users'
    });
  });

  it('handles a markerless chunk as output', () => {
    expect(parseBlock('Deprecated: ...')).toEqual({ kind: 'output', body: 'Deprecated: ...' });
  });
});

describe('fontSize', () => {
  it('reads a stored size and falls back to the default', () => {
    expect(fontSize('14')).toBe(14);
    expect(fontSize(null)).toBe(12);
    expect(fontSize('big')).toBe(12);
  });

  it('keeps the size inside the range', () => {
    expect(fontSize(String(FONT_MAX + 4))).toBe(FONT_MAX);
    expect(fontSize(String(FONT_MIN - 4))).toBe(FONT_MIN);
  });
});

describe('splitRatio', () => {
  it('defaults to an even split when nothing usable is stored', () => {
    expect(splitRatio(null)).toBe(0.5);
    expect(splitRatio('')).toBe(0.5);
    expect(splitRatio('wide')).toBe(0.5);
  });

  it('keeps a stored ratio inside the bounds', () => {
    expect(splitRatio('0.3')).toBe(0.3);
    expect(splitRatio('0')).toBe(SPLIT_MIN);
    expect(splitRatio('-1')).toBe(SPLIT_MIN);
    expect(splitRatio('1')).toBe(SPLIT_MAX);
    expect(splitRatio('Infinity')).toBe(SPLIT_MAX);
  });
});
