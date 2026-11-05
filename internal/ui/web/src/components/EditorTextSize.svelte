<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import Popover from '$components/Popover.svelte';
  import { tooltip } from '$lib/tooltip';
  import { editorFontSize, FONT_MAX, FONT_MIN } from '$stores/editorFont';
  import { m } from '../paraglide/messages.js';

  // aria-disabled at the limits: a disabled button would drop keyboard focus out of the panel.
  const step = 'px-2 py-0.5 text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100 aria-disabled:opacity-40 aria-disabled:hover:text-gray-500 transition-colors';

  let { iconClass = 'w-4 h-4' }: { iconClass?: string } = $props();
</script>

<Popover label={m.tinker_textSize()} align="right" width={168}>
  {#snippet triggerButton(toggle)}
    <button
      type="button"
      onclick={toggle}
      class="block text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
      use:tooltip={m.tinker_textSize()}
      aria-label={m.tinker_textSize()}
    >
      <Icon name="textSize" class={iconClass} />
    </button>
  {/snippet}
  {#snippet children()}
    <div class="flex items-center justify-between gap-3 px-3 py-2" data-testid="text-size">
      <span class="text-xs text-gray-600 dark:text-gray-300">{m.tinker_textSize()}</span>
      <div class="flex items-center rounded-full border border-gray-200 dark:border-lerd-border text-xs font-mono">
        <button type="button" class={step} aria-disabled={$editorFontSize <= FONT_MIN} onclick={() => ($editorFontSize = Math.max(FONT_MIN, $editorFontSize - 1))} aria-label={m.tinker_textSmaller()}>−</button>
        <span class="w-6 text-center tabular-nums text-gray-700 dark:text-gray-200">{$editorFontSize}</span>
        <button type="button" class={step} aria-disabled={$editorFontSize >= FONT_MAX} onclick={() => ($editorFontSize = Math.min(FONT_MAX, $editorFontSize + 1))} aria-label={m.tinker_textLarger()}>+</button>
      </div>
    </div>
  {/snippet}
</Popover>
