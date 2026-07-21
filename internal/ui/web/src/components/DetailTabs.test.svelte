<script lang="ts">
  import DetailTabs, { type TabItem } from './DetailTabs.svelte';

  interface Props {
    tabs: TabItem<string>[];
    active: string;
    onchange: (id: string) => void;
    withActions?: boolean;
  }
  let { tabs, active, onchange, withActions = false }: Props = $props();
</script>

{#snippet demoActions()}
  <button type="button">Toggle</button>
{/snippet}

<DetailTabs {tabs} {active} {onchange} actions={withActions ? demoActions : undefined} />
