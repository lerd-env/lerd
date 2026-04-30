<script lang="ts">
  import DashboardCard from './DashboardCard.svelte';

  import type { CardTone } from './DashboardCard.svelte';

  interface Props {
    title: string;
    withBadge?: boolean;
    withFooter?: boolean;
    tone?: CardTone;
  }
  let { title, withBadge = false, withFooter = false, tone }: Props = $props();
</script>

{#if withBadge && withFooter}
  {#snippet badge()}<span data-testid="badge">badge content</span>{/snippet}
  {#snippet footer()}<span data-testid="footer">footer content</span>{/snippet}
  <DashboardCard {title} {tone} {badge} {footer}>body content</DashboardCard>
{:else if withBadge}
  {#snippet badge()}<span data-testid="badge">badge content</span>{/snippet}
  <DashboardCard {title} {tone} {badge}>body content</DashboardCard>
{:else if withFooter}
  {#snippet footer()}<span data-testid="footer">footer content</span>{/snippet}
  <DashboardCard {title} {tone} {footer}>body content</DashboardCard>
{:else}
  <DashboardCard {title} {tone}>body content</DashboardCard>
{/if}
