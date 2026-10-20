<script lang="ts">
  import Modal from './Modal.svelte';

  interface Props {
    open: boolean;
    title: string;
    onclose: () => void;
    withFooter?: boolean;
    withActions?: boolean;
    size?: 'sm' | 'md' | 'lg' | 'xl';
    tall?: boolean;
  }
  let { open, title, onclose, withFooter = false, withActions = false, size = 'md', tall = false }: Props = $props();
</script>

{#if withActions}
  {#snippet actions()}<span data-testid="actions">A</span>{/snippet}
  <Modal {open} {title} {onclose} {size} {actions}>
    <div data-testid="body">BODY</div>
  </Modal>
{:else if withFooter}
  {#snippet footer()}<span data-testid="footer">F</span>{/snippet}
  <Modal {open} {title} {onclose} {size} {footer}>
    <div data-testid="body">BODY</div>
  </Modal>
{:else}
  <Modal {open} {title} {onclose} {size} {tall}>
    <div data-testid="body">BODY</div>
  </Modal>
{/if}
