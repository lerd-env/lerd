<script lang="ts">
  import Modal from './Modal.svelte';
  import Dropdown from './Dropdown.svelte';
  import Popover from './Popover.svelte';

  interface Props {
    open: boolean;
    title: string;
    onclose: () => void;
    withFooter?: boolean;
    withActions?: boolean;
    size?: 'sm' | 'md' | 'lg' | 'xl';
    tall?: boolean;
    withDropdown?: boolean;
    withPopover?: boolean;
  }
  let { open, title, onclose, withFooter = false, withActions = false, size = 'md', tall = false, withDropdown = false, withPopover = false }: Props = $props();
</script>

{#if withDropdown}
  <Modal {open} {title} {onclose} {size}>
    <Dropdown value="a" options={[{ value: 'a', label: 'Alpha' }, { value: 'b', label: 'Beta' }]} onchange={() => {}} />
  </Modal>
{:else if withPopover}
  <Modal {open} {title} {onclose} {size}>
    <Popover label="Open">
      {#snippet trigger()}open{/snippet}
      {#snippet children()}<button type="button">panel item</button>{/snippet}
    </Popover>
  </Modal>
{:else if withActions}
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
