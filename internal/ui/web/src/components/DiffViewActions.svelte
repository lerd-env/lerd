<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '$components/Icon.svelte';
  import EditorTextSize from '$components/EditorTextSize.svelte';
  import { tooltip } from '$lib/tooltip';
  import { m } from '../paraglide/messages.js';

  interface Props {
    fullscreen: boolean;
    unified?: boolean;
    // A single file has no second pane to lay out, so it leaves the toggle out.
    split?: boolean;
    // Buttons of the caller's own, drawn before full screen.
    extra?: Snippet;
  }
  let { fullscreen = $bindable(), unified = $bindable(false), split = true, extra }: Props = $props();
</script>

<!-- Same gap and icon size as the modal's own close button beside them. -->
<div class="flex items-center gap-3">
  <EditorTextSize iconClass="w-5 h-5" />
  {#if split}
    <button
      type="button"
      onclick={() => (unified = !unified)}
      class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
      use:tooltip={unified ? m.gitDiff_sideBySide() : m.gitDiff_unified()}
      aria-label={unified ? m.gitDiff_sideBySide() : m.gitDiff_unified()}
    >
      <Icon name={unified ? 'splitHorizontal' : 'splitVertical'} class="w-5 h-5" />
    </button>
  {/if}
  {@render extra?.()}
  <button
    type="button"
    onclick={() => (fullscreen = !fullscreen)}
    class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
    use:tooltip={fullscreen ? m.gitDiff_exitFullscreen() : m.tinker_fullscreenTitle()}
    aria-label={fullscreen ? m.gitDiff_exitFullscreen() : m.tinker_fullscreenTitle()}
  >
    <Icon name={fullscreen ? 'minimize' : 'maximize'} class="w-5 h-5" />
  </button>
</div>
