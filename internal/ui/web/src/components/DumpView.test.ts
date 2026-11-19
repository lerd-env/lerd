import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import DumpView from './DumpView.svelte';
import { parseDump } from '$lib/dump-parser';

// jsdom has no layout engine, so the check is on the class that makes a nested
// block take a full row of its flex-wrap parent instead of sitting beside the key.
function nestedBlocks(text: string) {
  const parsed = parseDump(text);
  if (!parsed.ok) throw new Error('fixture did not parse');
  const { container } = render(DumpView, { props: { node: parsed.nodes[0] } });
  return [...container.querySelectorAll('div.border-l')].slice(1);
}

describe('DumpView', () => {
  it('puts a nested array on its own line under its key', () => {
    const blocks = nestedBlocks(`array:1 [
  "summary" => array:1 [
    "counts" => 50
  ]
]`);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]).toHaveClass('basis-full');
  });

  it('puts a nested object on its own line under its property', () => {
    const blocks = nestedBlocks(`App\\Outer {#1
  +inner: App\\Inner {#2
    +name: "x"
  }
}`);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]).toHaveClass('basis-full');
  });
});
