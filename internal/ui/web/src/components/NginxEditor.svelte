<script lang="ts">
  import MonacoEditor from './MonacoEditor.svelte';

  interface Props {
    value: string;
    readOnly?: boolean;
    onChange?: (next: string) => void;
    /** 'yaml' for lerd's own web server's settings files. */
    language?: 'nginx' | 'yaml';
  }
  let { value = $bindable(''), readOnly = false, onChange, language = 'nginx' }: Props = $props();
</script>

<MonacoEditor bind:value {readOnly} {onChange} {language} />
