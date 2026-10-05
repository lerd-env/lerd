<script lang="ts">
  import type { Frame } from '$lib/sourceLabel';
  import SourcePath from './SourcePath.svelte';

  // A stack trace, innermost first, the app's own frames in full colour.
  let { trace }: { trace: Frame[] } = $props();
</script>

<ol class="font-mono space-y-0.5">
  {#each trace as frame, i (i)}
    {@const app = !frame.file?.includes('/vendor/')}
    <li class={app ? 'text-gray-700 dark:text-gray-200' : 'text-gray-500 dark:text-gray-400'}>
      {#if frame.func}<span class={app ? 'font-semibold' : ''}>{frame.func}</span> · {/if}
      {#if frame.file}<SourcePath file={frame.file} line={frame.line} muted={!app} />{/if}
    </li>
  {/each}
</ol>
