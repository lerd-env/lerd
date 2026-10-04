<script lang="ts">
  import { openInEditor } from '$lib/editor';
  import { tooltip } from '$lib/tooltip';
  import CopyButton from './CopyButton.svelte';
  import { sites } from '$stores/sites';
  import { relativeTo } from '$lib/sourceLabel';
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
  }
  let { file, line, muted = false, short = false, label = '' }: Props = $props();

  const reference = $derived(line ? `${file}:${line}` : file);
  // A path inside a site reads from the project's root; one outside keeps its
  // full form, shortened where the row is narrow.
  const relative = $derived(relativeTo(file, $sites.map((s) => s.path).filter((p): p is string => !!p)));
  const shown = $derived(label || (relative !== file ? relative : short ? shorten(file) : file));

  function shorten(path: string): string {
    const parts = path.split('/');
    return parts.length <= 3 ? path : '…/' + parts.slice(-3).join('/');
  }
</script>

<span class="group/path inline-flex items-center gap-1 max-w-full align-middle">
  <button
    type="button"
    class="font-mono text-left hover:underline {muted
      ? 'hover:text-gray-600 dark:hover:text-gray-300'
      : 'text-lerd-red'} {short ? 'min-w-0 truncate' : 'break-all'}"
    onclick={() => openInEditor(file, line ?? 1)}
    use:tooltip={`${m.queries_openInEditor()} — ${reference}`}
  >{shown}{#if line}:{line}{/if}</button>
  <CopyButton
    text={reference}
    label={m.queries_copyPath()}
    tone="faint"
    size="w-3.5 h-3.5"
    class="opacity-0 group-hover/path:opacity-100 focus-visible:opacity-100 transition-opacity"
  />
</span>
