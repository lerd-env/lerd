<script lang="ts" module>
  export interface GraphQLTypeInfo {
    kind: 'object' | 'input' | 'enum' | 'union' | 'scalar';
    fields?: { name: string; type: string; args?: { name: string; type: string }[] }[];
    values?: string[];
    types?: string[];
    file?: string;
    line?: number;
  }
  // named is the type a list or non-null wraps, [Case!]! read as Case.
  export const named = (type: string) => type.replace(/[[\]!]/g, '');
</script>

<script lang="ts">
  import SourcePath from './SourcePath.svelte';
  import Icon from './Icon.svelte';
  import GraphQLType from './GraphQLType.svelte';
  import { m } from '../paraglide/messages.js';

  // One type of the schema a request used, folded until opened: its fields
  // with their arguments and types, each type in turn opening the same way.
  // path holds the types already open above this one, so a type the schema
  // circles back to is marked rather than opened again without end.
  interface Props {
    type: string;
    types: Record<string, GraphQLTypeInfo>;
    path?: string[];
  }
  let { type, types, path = [] }: Props = $props();
  const info = $derived(types[named(type)]);
  const circular = $derived(path.includes(named(type)));
  const openable = $derived(!circular && !!info && (!!info.fields?.length || !!info.values?.length || !!info.types?.length));
  const inner = $derived([...path, named(type)]);
  let open = $state(false);
</script>

<span class="inline-flex items-center gap-1 font-mono text-[11px] text-amber-700 dark:text-amber-300">
  {#if openable}
    <button type="button" aria-expanded={open} onclick={() => (open = !open)} class="text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"><Icon name="chevron" class="w-3 h-3 transition-transform {open ? '' : '-rotate-90'}" /></button>
  {/if}
  {#if info?.file}<SourcePath file={info.file} line={info.line} label={type} bare dotted />{:else}{type}{/if}
  {#if info && info.kind !== 'object' && info.kind !== 'scalar'}<span class="text-gray-400">{info.kind}</span>{/if}
  {#if circular}<span class="text-gray-400" title={m.requests_graphql_circular()}>↻</span>{/if}
</span>
{#if open && info}
  <div class="basis-full w-full ml-4 mt-1 pl-3 border-l border-gray-200 dark:border-lerd-border space-y-0.5">
    {#each info.fields ?? [] as f (f.name)}
      <div class="text-[11px]">
        <span class="font-mono text-gray-800 dark:text-gray-100">{f.name}</span>{#if f.args?.length}<span class="font-mono text-gray-400">({f.args.map((a) => `${a.name}: ${a.type}`).join(', ')})</span>{/if}<span class="text-gray-400 mr-1">:</span><GraphQLType type={f.type} {types} path={inner} />
      </div>
    {/each}
    {#each info.values ?? [] as v (v)}<div class="font-mono text-[11px] text-gray-700 dark:text-gray-200">{v}</div>{/each}
    {#each info.types ?? [] as t (t)}<div class="text-[11px]"><GraphQLType type={t} {types} path={inner} /></div>{/each}
  </div>
{/if}
