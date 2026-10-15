<script lang="ts">
  import { apiUrl } from '$lib/api';
  import Icon from './Icon.svelte';

  interface Props {
    icon?: string;
  }
  let { icon }: Props = $props();

  const mask = $derived(icon ? `url(${JSON.stringify(apiUrl(icon))}) center / contain no-repeat` : '');
</script>

<!-- A mask, not inline markup: the plugin's SVG never enters the dashboard's DOM,
     and the glyph still takes the rail's currentColor like the built-in icons. -->
{#if icon}
  <span class="block w-5 h-5 bg-current" style:mask style:-webkit-mask={mask} aria-hidden="true"></span>
{:else}
  <Icon name="cube" />
{/if}
