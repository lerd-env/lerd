<script lang="ts">
  import type { GroupLabel } from '$lib/eventGroup';
  import { sites, siteDomainForName } from '$stores/sites';

  interface Props {
    label: GroupLabel;
  }
  let { label }: Props = $props();

  // A site name that resolves to a linked site opens that site's Debug tab;
  // anything else (an unregistered name, the empty "(unknown)" case, and the
  // branch shown on its own inside a site) stays plain text.
  const domain = $derived(siteDomainForName($sites, label.site));
  const suffix = $derived(label.site && label.branch ? '@' + label.branch : '');
</script>

<span class="text-sm truncate">
  {#if label.site || label.branch}<span
      >[{#if label.site && domain}<a
          href="#sites/{domain}/dumps"
          class="underline decoration-dotted underline-offset-2 hover:text-lerd-red">{label.site}</a
        >{:else if label.site}{label.site}{:else}{label.branch}{/if}{suffix}]
    </span>{/if}{label.text}
</span>
