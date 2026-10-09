<script lang="ts">
  import Dropdown, { type DropdownOption } from './Dropdown.svelte';

  interface Props {
    label?: string;
    value?: string;
    options?: Array<string | DropdownOption>;
    disabled?: boolean;
    inherited?: boolean;
    inheritedSuffix?: string;
    title?: string;
    placeholder?: string;
    width?: 'auto' | 'full';
    align?: 'left' | 'right';
    searchable?: boolean;
    onchange: (v: string) => void;
  }
  let {
    label = '',
    value = '',
    options = [],
    disabled = false,
    inherited = false,
    inheritedSuffix = '',
    title = '',
    placeholder = '',
    width = 'auto',
    align = 'left',
    searchable = false,
    onchange
  }: Props = $props();
</script>

<Dropdown
  {label}
  {value}
  {options}
  {disabled}
  {inherited}
  {inheritedSuffix}
  {title}
  {placeholder}
  {width}
  {align}
  {searchable}
  {onchange}
/>
