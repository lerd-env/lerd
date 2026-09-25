<script lang="ts">
  import { desktopSuggestion, keepCurrentTheme, useSuggestedTheme } from '$stores/palettes';
  import { bannersAllowed } from '$stores/setup';
  import { m } from '../paraglide/messages.js';
  import BottomBanner from './BottomBanner.svelte';
</script>

{#if $desktopSuggestion && $bannersAllowed}
  <BottomBanner
    title={m.theme_suggest_title({ name: $desktopSuggestion.name })}
    subtitle={m.theme_suggest_subtitle()}
    actionLabel={m.theme_suggest_use()}
    dismissLabel={m.theme_suggest_keep()}
    onAction={useSuggestedTheme}
    onDismiss={keepCurrentTheme}
  >
    {#snippet icon()}
      <svg class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.34M11 7.34l1.66-1.66a2 2 0 012.83 0l2.83 2.83a2 2 0 010 2.83L10 19.66M7 17h.01"/>
      </svg>
    {/snippet}
  </BottomBanner>
{/if}
