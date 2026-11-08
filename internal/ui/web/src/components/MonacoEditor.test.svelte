<script lang="ts">
  import MonacoEditor from './MonacoEditor.svelte';
  import type * as Monaco from 'monaco-editor';
  import type { MonacoModule } from '../lib/monaco';

  interface Props {
    value?: string;
    language?: string;
    readOnly?: boolean;
    onChange?: (next: string) => void;
    onReady?: (ctx: { editor: Monaco.editor.IStandaloneCodeEditor; monaco: MonacoModule }) => void;
  }
  let { value = $bindable(''), language, readOnly, onChange, onReady }: Props = $props();
</script>

<MonacoEditor bind:value {language} {readOnly} {onChange} {onReady} />
