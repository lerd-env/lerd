import { describe, it, expect } from 'vitest';
import { diffLines, addedLineNumbers } from './diff';

describe('diffLines', () => {
  it('marks every line as context when inputs match', () => {
    const out = diffLines('A\nB\nC', 'A\nB\nC');
    expect(out).toEqual([
      { op: ' ', line: 'A' },
      { op: ' ', line: 'B' },
      { op: ' ', line: 'C' }
    ]);
  });

  it('emits a single insertion', () => {
    const out = diffLines('A\nC', 'A\nB\nC');
    expect(out).toEqual([
      { op: ' ', line: 'A' },
      { op: '+', line: 'B' },
      { op: ' ', line: 'C' }
    ]);
  });

  it('emits a single deletion', () => {
    const out = diffLines('A\nB\nC', 'A\nC');
    expect(out).toEqual([
      { op: ' ', line: 'A' },
      { op: '-', line: 'B' },
      { op: ' ', line: 'C' }
    ]);
  });

  it('emits a single replacement as delete + insert', () => {
    const out = diffLines('DB_HOST=localhost', 'DB_HOST=127.0.0.1');
    expect(out).toEqual([
      { op: '-', line: 'DB_HOST=localhost' },
      { op: '+', line: 'DB_HOST=127.0.0.1' }
    ]);
  });

  it('treats empty input as fully inserted / deleted', () => {
    expect(diffLines('', 'A\nB')).toEqual([
      { op: '+', line: 'A' },
      { op: '+', line: 'B' }
    ]);
    expect(diffLines('A\nB', '')).toEqual([
      { op: '-', line: 'A' },
      { op: '-', line: 'B' }
    ]);
  });

  it('returns an empty array for identical empty inputs', () => {
    expect(diffLines('', '')).toEqual([]);
  });
});

describe('addedLineNumbers', () => {
  it('reports the current-buffer lines inserted since original', () => {
    const original = 'DB_HOST=x\nDB_DATABASE=app';
    const current = 'DB_HOST=x\nDB_PORT=5432\nDB_DATABASE=app';
    expect(addedLineNumbers(original, current)).toEqual([2]);
  });

  it('does not shift onto surviving lines when an added line is removed again', () => {
    const original = 'A=1\nB=2';
    // User inserted two lines then deleted the first insertion; only the
    // remaining insertion should be marked, on its new line number.
    expect(addedLineNumbers(original, 'A=1\nNEW2=y\nB=2')).toEqual([2]);
  });

  it('is empty when nothing changed', () => {
    expect(addedLineNumbers('A=1\nB=2', 'A=1\nB=2')).toEqual([]);
  });
});
