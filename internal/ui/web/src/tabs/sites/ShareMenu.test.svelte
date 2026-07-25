<script lang="ts">
  import ShareMenu from './ShareMenu.svelte';
  import type { Site } from '$stores/sites';

  interface Props {
    site: Site;
    activeWorktreeBranch?: string;
    lanOn?: boolean;
    lanBusy?: boolean;
    lanUrl?: string;
    onToggleLan?: () => void;
  }
  let {
    site,
    activeWorktreeBranch = '',
    lanOn = false,
    lanBusy = false,
    lanUrl = '',
    onToggleLan = () => {}
  }: Props = $props();
</script>

<ShareMenu {site} {activeWorktreeBranch} {lanOn} {lanBusy} {lanUrl} {onToggleLan} />
