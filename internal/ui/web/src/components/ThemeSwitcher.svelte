<script lang="ts">
  import Icon, { type IconName } from '$components/Icon.svelte';
  import Popover from '$components/Popover.svelte';
  import { theme, type Theme } from '$stores/theme';
  import { m } from '../paraglide/messages.js';

  interface Props {
    size?: 'sm' | 'md';
    align?: 'left' | 'right';
  }
  let { size = 'sm', align = 'left' }: Props = $props();

  const modes: Theme[] = ['light', 'dark', 'auto'];
  const icons: Record<Theme, IconName> = { light: 'sun', dark: 'moon', auto: 'contrast' };

  const labels = $derived<Record<Theme, string>>({
    light: m.theme_light(),
    dark: m.theme_dark(),
    auto: m.theme_auto()
  });

  // The rail icon shows the mode in effect and the tooltip names it. For auto it
  // also names what the system resolved to, since the icon alone cannot tell the
  // user whether they are currently on light or dark.
  const systemDark = $derived(
    typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: dark)').matches
  );
  const triggerLabel = $derived(
    $theme === 'auto' ? `${labels.auto} (${systemDark ? labels.dark : labels.light})` : labels[$theme]
  );
</script>

<Popover label={triggerLabel} {size} {align} width={180}>
  {#snippet trigger()}
    <Icon name={icons[$theme]} class="w-5 h-5" />
  {/snippet}
  {#snippet children(close: () => void)}
    <ul class="py-1">
      {#each modes as mode (mode)}
        <li>
          <button
            type="button"
            onclick={() => {
              theme.set(mode);
              close();
            }}
            class="flex w-full items-center gap-2.5 px-3 py-1.5 text-left text-xs capitalize transition-colors hover:bg-gray-50 dark:hover:bg-white/5 {$theme ===
            mode
              ? 'text-gray-900 dark:text-white'
              : 'text-gray-500 dark:text-gray-400'}"
          >
            <Icon name={icons[mode]} class="w-3.5 h-3.5 shrink-0" />
            <span class="flex-1">{labels[mode]}</span>
            {#if $theme === mode}
              <Icon name="check" class="w-3.5 h-3.5 shrink-0 text-lerd-red" />
            {/if}
          </button>
        </li>
      {/each}
    </ul>
  {/snippet}
</Popover>
