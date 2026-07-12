<script lang="ts">
  import WorktreeDBDropModal from './WorktreeDBDropModal.svelte';

  interface Props {
    open: boolean;
    branch: string;
    database?: string;
    onclose: () => void;
    onconfirm: () => void;
  }
  let { open, branch, database = '', onclose, onconfirm }: Props = $props();
</script>

<WorktreeDBDropModal {open} {branch} {database} {onclose} {onconfirm} />
