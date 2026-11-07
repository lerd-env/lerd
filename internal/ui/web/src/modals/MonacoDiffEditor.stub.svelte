<script lang="ts">
  import type { Hunk } from '$lib/gitDiff';
  let {
    original,
    modified,
    language,
    ignoreSpace,
    sideBySide,
    hunkAction,
    splitRatio,
    onSplit
  }: {
    original: string;
    modified: string;
    language: string;
    ignoreSpace: boolean;
    sideBySide: boolean;
    hunkAction?: { label: string; unstage: boolean; run: (h: Hunk) => void };
    splitRatio?: number;
    onSplit?: (ratio: number) => void;
  } = $props();
  const hunk = (): Hunk => ({ orig_start: 1, orig_end: 1, mod_start: 1, mod_end: 1, original, modified });
</script>

<div
  data-testid="diff"
  data-original={original}
  data-modified={modified}
  data-language={language}
  data-ignore-space={String(ignoreSpace)}
  data-side-by-side={String(sideBySide)}
  data-split={String(splitRatio)}
></div>
<button type="button" onclick={() => onSplit?.(0.3)}>drag split</button>
{#if hunkAction}<button type="button" onclick={() => hunkAction.run(hunk())}>{hunkAction.label}</button>{/if}
