import { describe, it, expect } from 'vitest';
import { diffStats, languageFor } from './gitDiff';

describe('languageFor', () => {
  it('picks a grammar lerd ships and falls back to plain text', () => {
    expect(languageFor('app/Models/User.php')).toBe('php');
    expect(languageFor('.env')).toBe('dotenv');
    expect(languageFor('deploy/.env.production')).toBe('dotenv');
    expect(languageFor('php/custom.ini')).toBe('ini');
    expect(languageFor('nginx/site.conf')).toBe('nginx');
    expect(languageFor('.lerd.yaml')).toBe('plaintext');
    // A folder named like a grammar says nothing about the file in it.
    expect(languageFor('.env.d/notes.txt')).toBe('plaintext');
  });
});

// Changes as Monaco reports them, counted the way git would.
describe('diffStats', () => {
  const change = (os: number, oe: number, ms: number, me: number) => ({
    originalStartLineNumber: os,
    originalEndLineNumber: oe,
    modifiedStartLineNumber: ms,
    modifiedEndLineNumber: me
  });

  it('counts a new file by its own lines, not the empty side or the blank after the last newline', () => {
    expect(diffStats([change(1, 1, 1, 2)], '', '{"require":{}}\n')).toEqual({ added: 1, removed: 0 });
  });

  it('counts a deleted file the same way', () => {
    expect(diffStats([change(1, 3, 1, 1)], 'a\nb\n', '')).toEqual({ added: 0, removed: 2 });
  });

  it('counts an edit in the middle as git does', () => {
    expect(diffStats([change(2, 2, 2, 3)], 'a\nb\nc\n', 'a\nB\nB2\nc\n')).toEqual({ added: 2, removed: 1 });
  });

  it('keeps a last line that has no newline', () => {
    expect(diffStats([change(0, 0, 2, 2)], 'a', 'a\nb')).toEqual({ added: 1, removed: 0 });
  });
});
