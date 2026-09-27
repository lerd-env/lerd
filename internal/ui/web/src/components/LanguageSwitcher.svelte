<script lang="ts">
  import { locale, changeLocale, LOCALES, LOCALE_LABELS, LOCALE_CODES, type Locale } from '$stores/locale';
  import Dropdown from './Dropdown.svelte';

  interface Props {
    variant?: 'compact' | 'select';
  }
  let { variant = 'select' }: Props = $props();

  const options = LOCALES.map((l) => ({ value: l, label: LOCALE_LABELS[l] }));
</script>

{#if variant === 'compact'}
  <div class="flex items-center rounded-md border border-gray-200 dark:border-lerd-border overflow-hidden">
    {#each LOCALES as l (l)}
      <button
        title={LOCALE_LABELS[l]}
        onclick={() => changeLocale(l)}
        class="px-2 py-1.5 text-[10px] font-medium transition-colors {$locale === l
          ? 'bg-gray-200 dark:bg-white/10 text-gray-900 dark:text-white'
          : 'text-gray-500 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-white/5'}"
      >{LOCALE_CODES[l]}</button>
    {/each}
  </div>
{:else}
  <Dropdown value={$locale} {options} onchange={(v) => changeLocale(v as Locale)} />
{/if}
