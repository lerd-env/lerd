<script lang="ts">
  import { openInEditor } from '$lib/editor';
  import { tooltip } from '$lib/tooltip';
  import CopyButton from './CopyButton.svelte';
  import { sites } from '$stores/sites';
  import { editorTitle } from '$stores/editors';
  import { relativeTo, shortClass } from '$lib/sourceLabel';
  import { m } from '../paraglide/messages.js';

  interface Props {
    file: string;
    line?: number;
    // Vendor frames and header paths sit behind the app's own code.
    muted?: boolean;
    // Keep only the tail of a long path where the row has no width to spare.
    short?: boolean;
    // label replaces the path, the class a call was made in, say.
    label?: string;
    // bare leaves the line out of the text, where a colon means something
    // else, a middleware's parameters; the editor still opens at it.
    bare?: boolean;
    // dotted reads as the surrounding text with a dotted underline, for a link
    // among names that mostly are not.
    dotted?: boolean;
  }
  let { file, line, muted = false, short = false, label = '', bare = false, dotted = false }: Props = $props();

  const reference = $derived(line ? `${file}:${line}` : file);
  // A path inside a site reads from the project's root; one outside keeps its
  // full form, shortened where the row is narrow.
  const relative = $derived(relativeTo(file, $sites.map((s) => s.path).filter((p): p is string => !!p)));
  // A class label reads by the class's own name; the full name is in the tooltip.
  const shown = $derived(label ? shortClass(label) : relative !== file ? relative : short ? shorten(file) : file);

  function shorten(path: string): string {
    const parts = path.split('/');
    return parts.length <= 3 ? path : '…/' + parts.slice(-3).join('/');
  }
</script>

<span class="inline-flex items-center gap-1 max-w-full align-middle">
  <button
    type="button"
    class="font-mono text-left {dotted
      ? 'underline decoration-dotted decoration-lerd-red underline-offset-2 hover:decoration-solid'
      : muted
        ? 'hover:underline hover:text-gray-600 dark:hover:text-gray-300'
        : 'hover:underline text-lerd-red'} {short ? 'min-w-0 truncate' : 'break-all'}"
    onclick={() => openInEditor(file, line ?? 1)}
    use:tooltip={$editorTitle(file)}
  >{shown}{#if line && !bare}:{line}{/if}</button>
  <CopyButton
    text={reference}
    label={m.queries_copyPath()}
    tone="faint"
    size="w-3.5 h-3.5"
  />
</span>
