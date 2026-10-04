import { describe, it, expect } from 'vitest';
import { toDumpNodes, splitLogContext } from './structured';

describe('toDumpNodes', () => {
  it('turns JSON text and plain objects into a navigable tree', () => {
    expect(toDumpNodes('{"items":[{"sku":"A-1"}],"total":19.9}')).toEqual([
      {
        kind: 'array', count: 2, items: [
          { key: '"items"', value: { kind: 'array', count: 1, items: [{ key: '0', value: { kind: 'array', count: 1, items: [{ key: '"sku"', value: { kind: 'scalar', type: 'string', value: '"A-1"' } }] } }] } },
          { key: '"total"', value: { kind: 'scalar', type: 'number', value: '19.9' } }
        ]
      }
    ]);
    expect(toDumpNodes({ ok: true })?.[0]).toMatchObject({ kind: 'array', count: 1 });
  });

  it('parses VarDumper text and leaves prose and scalars alone', () => {
    expect(toDumpNodes('array:1 [\n  "visits" => 6\n]')?.[0]).toMatchObject({ kind: 'array', count: 1 });
    expect(toDumpNodes('Welcome back')).toBeNull();
    expect(toDumpNodes('{not json}')).toBeNull();
    expect(toDumpNodes(42)).toBeNull();
  });
});

describe('splitLogContext', () => {
  it('takes the context and extra a line formatter appended off the message', () => {
    expect(splitLogContext('demo page served {"visits":1} []')).toEqual({ text: 'demo page served', context: { visits: 1 } });
    expect(splitLogContext('user {id} saved {"user":{"id":7}}')).toEqual({ text: 'user {id} saved', context: { user: { id: 7 } } });
    expect(splitLogContext('No hint path defined for [layouts].')).toEqual({ text: 'No hint path defined for [layouts].' });
  });
});

