<script lang="ts">
  import { PRERELEASE_LABEL } from '$stores/phpVersions';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    version: string;
    patch?: string;
    running: boolean;
    isDefault: boolean;
    updateAvailable?: boolean;
    prerelease?: boolean;
    selected: boolean;
    onselect: () => void;
  }
  let {
    version,
    patch,
    running,
    isDefault,
    updateAvailable = false,
    prerelease = false,
    selected,
    onselect
  }: Props = $props();

  // Show the full build when known ("8.5.8"), with the patch tail dimmed so it
  // reads as one number; fall back to the minor until the probe lands. A
  // prerelease build ("8.6.0beta2") is too wide to sit next to the beta mark,
  // so the card keeps the minor and the tooltip carries the full string.
  const display = $derived(patch || version);
  const minor = $derived(display.split('.').slice(0, 2).join('.'));
  const tail = $derived(
    prerelease || display.split('.').length < 3 ? '' : '.' + display.split('.').slice(2).join('.')
  );
</script>

<button
  type="button"
  onclick={onselect}
  title={'PHP ' + display + ' — ' + (running ? m.common_running() : m.common_stopped()) + (isDefault ? ' · ' + m.common_default() : '') + (updateAvailable ? ' · ' + m.system_php_baseUpdateHint() : '') + (prerelease ? ' · ' + PRERELEASE_LABEL : '')}
  class="shrink-0 flex items-center gap-2 h-8 px-3 rounded-lg border text-sm transition-colors {selected
    ? 'border-lerd-red/60 bg-lerd-red/10 text-gray-900 dark:text-white'
    : 'border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card text-gray-700 dark:text-gray-300 hover:border-gray-300 dark:hover:border-gray-600'}"
>
  <span
    class="w-2 h-2 rounded-full shrink-0 {running ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-gray-600'}"
    role="img"
    aria-label={running ? m.common_running() : m.common_stopped()}
  ></span>
  <span class="font-mono font-medium tabular-nums">
    {minor}<span class="text-gray-500 dark:text-gray-400">{tail}</span>
  </span>
  {#if isDefault}
    <svg class="w-3.5 h-3.5 shrink-0 text-lerd-red" fill="currentColor" viewBox="0 0 20 20" aria-label={m.common_default()}>
      <path d="M10 1.5l2.6 5.27 5.82.85-4.21 4.1.99 5.78L10 14.77l-5.2 2.73.99-5.78L1.58 7.62l5.82-.85L10 1.5z" />
    </svg>
  {/if}
  {#if prerelease}
    <span class="shrink-0 text-[9px] font-bold uppercase tracking-wide leading-none rounded px-1 py-0.5 text-amber-700 bg-amber-100 dark:text-amber-300 dark:bg-amber-400/15">
      {PRERELEASE_LABEL}
    </span>
  {/if}
  {#if updateAvailable}
    <svg class="w-3.5 h-3.5 shrink-0 text-yellow-600 dark:text-yellow-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-label={m.system_php_baseUpdate()}>
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4" />
    </svg>
  {/if}
</button>
