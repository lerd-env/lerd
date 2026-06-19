// Minimal LSP client for the tinker editor. The Go bridge (/api/lsp/php)
// spawns phpantom_lsp and translates its stdio Content-Length framing into one
// JSON-RPC message per WebSocket frame, so we can speak JSON-RPC directly here
// without pulling in the (very heavy) monaco-languageclient + vscode shim
// stack. We register Monaco completion/hover/signature providers that proxy to
// the server, and surface its diagnostics as editor markers.
//
// Tinker buffers are headerless PHP (no `<?php`), which phpantom can't parse.
// We present the document to the server with a synthetic leading `<?php` line
// and offset positions by it, so the editor stays headerless while the server
// sees valid PHP. The synthetic line is line 0 (LSP, 0-based); the user's
// first line (Monaco line 1) maps to LSP line 1, making the line mapping a
// straight identity in both directions.
import type { MonacoModule } from '$lib/monaco';
import type * as Monaco from 'monaco-editor';
import { wsUrl } from '$lib/api';

const DOC_PREFIX = '<?php\n';
const REQUEST_TIMEOUT_MS = 5000;

// LSP CompletionItemKind (1-based) -> Monaco kind name. Names line up with
// monaco.languages.CompletionItemKind, so we resolve the enum at runtime.
const LSP_COMPLETION_KIND = [
  'Text', 'Text', 'Method', 'Function', 'Constructor', 'Field', 'Variable',
  'Class', 'Interface', 'Module', 'Property', 'Unit', 'Value', 'Enum',
  'Keyword', 'Snippet', 'Color', 'File', 'Reference', 'Folder', 'EnumMember',
  'Constant', 'Struct', 'Event', 'Operator', 'TypeParameter'
] as const;

export interface PhpLspHandle {
  dispose(): void;
}

interface Pending {
  resolve: (v: unknown) => void;
  reject: (e: unknown) => void;
  timer: ReturnType<typeof setTimeout>;
}

export function attachPhpLsp(opts: {
  monaco: MonacoModule;
  editor: Monaco.editor.IStandaloneCodeEditor;
  domain: string;
  branch?: string;
  onStatus?: (status: 'connecting' | 'ready' | 'unavailable') => void;
}): PhpLspHandle {
  const { monaco, editor, domain, branch, onStatus } = opts;

  const params = new URLSearchParams({ domain });
  if (branch) params.set('branch', branch);
  const ws = new WebSocket(wsUrl(`/api/lsp/php?${params.toString()}`));

  let disposed = false;
  let nextId = 1;
  let docVersion = 1;
  let documentUri = '';
  const pending = new Map<number, Pending>();
  const notificationHandlers = new Map<string, (params: any) => void>();
  const disposables: Monaco.IDisposable[] = [];
  let rootResolve: ((root: string) => void) | null = null;
  const rootReady = new Promise<string>((r) => { rootResolve = r; });
  // Set true only after textDocument/didOpen is sent, so we never push a
  // didChange for a document the server hasn't been told about yet.
  let opened = false;

  onStatus?.('connecting');

  // ---- position mapping (Monaco 1-based <-> LSP 0-based, +1 synthetic line) ----
  const toLspPos = (p: Monaco.IPosition) => ({ line: p.lineNumber, character: p.column - 1 });
  const fromLspLine = (line: number) => Math.max(1, line);
  const fromLspRange = (r: any): Monaco.IRange => ({
    startLineNumber: fromLspLine(r.start.line),
    startColumn: r.start.character + 1,
    endLineNumber: fromLspLine(r.end.line),
    endColumn: r.end.character + 1
  });

  // ---- transport ----
  function rawSend(obj: unknown) {
    if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj));
  }
  function request<T = any>(method: string, p: unknown): Promise<T> {
    const id = nextId++;
    rawSend({ jsonrpc: '2.0', id, method, params: p });
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        pending.delete(id);
        reject(new Error(`lsp ${method} timed out`));
      }, REQUEST_TIMEOUT_MS);
      pending.set(id, { resolve: resolve as (v: unknown) => void, reject, timer });
    });
  }
  function notify(method: string, p: unknown) {
    rawSend({ jsonrpc: '2.0', method, params: p });
  }

  ws.onmessage = (ev) => {
    let msg: any;
    try { msg = JSON.parse(ev.data); } catch { return; }

    if (msg.type === 'lerd-root') {
      rootResolve?.(msg.root);
      return;
    }
    // Response to one of our requests.
    if (msg.id !== undefined && msg.method === undefined) {
      const p = pending.get(msg.id);
      if (p) {
        pending.delete(msg.id);
        clearTimeout(p.timer);
        if (msg.error) p.reject(msg.error);
        else p.resolve(msg.result);
      }
      return;
    }
    // Server -> client request or notification.
    if (msg.method) {
      const handler = notificationHandlers.get(msg.method);
      if (handler) handler(msg.params);
      if (msg.id !== undefined) {
        // Satisfy server-initiated requests so it doesn't stall. We accept
        // capability registrations and report empty configuration.
        const result = msg.method === 'workspace/configuration'
          ? (msg.params?.items ?? []).map(() => null)
          : null;
        rawSend({ jsonrpc: '2.0', id: msg.id, result });
      }
    }
  };

  ws.onerror = () => { if (!disposed) onStatus?.('unavailable'); };
  ws.onclose = () => {
    if (disposed) return;
    onStatus?.('unavailable');
    const model = editor.getModel();
    if (model) monaco.editor.setModelMarkers(model, 'phpantom', []);
  };

  // ---- diagnostics ----
  notificationHandlers.set('textDocument/publishDiagnostics', (p: any) => {
    if (!p || decodeURIComponent(p.uri ?? '') !== decodeURIComponent(documentUri)) return;
    const model = editor.getModel();
    if (!model) return;
    const sev = monaco.MarkerSeverity;
    const markers = (p.diagnostics ?? [])
      // Drop anything the server pins to the synthetic `<?php` line (LSP line
      // 0); user content always starts at LSP line 1.
      .filter((d: any) => d.range?.start?.line >= 1)
      .map((d: any) => {
        const r = fromLspRange(d.range);
        return {
          severity: d.severity === 1 ? sev.Error : d.severity === 2 ? sev.Warning : d.severity === 3 ? sev.Info : sev.Hint,
          message: d.message,
          source: d.source,
          startLineNumber: r.startLineNumber,
          startColumn: r.startColumn,
          endLineNumber: r.endLineNumber,
          endColumn: r.endColumn
        };
      });
    monaco.editor.setModelMarkers(model, 'phpantom', markers);
  });

  // ---- document sync ----
  function fullText(): string {
    return DOC_PREFIX + (editor.getModel()?.getValue() ?? '');
  }
  const modelListener = editor.onDidChangeModelContent(() => {
    if (!opened) return;
    notify('textDocument/didChange', {
      textDocument: { uri: documentUri, version: ++docVersion },
      contentChanges: [{ text: fullText() }]
    });
  });
  disposables.push(modelListener);

  // ---- completion / hover / signature providers ----
  function toMarkdown(doc: any): Monaco.IMarkdownString | undefined {
    if (!doc) return undefined;
    if (typeof doc === 'string') return { value: doc };
    if (doc.value) return { value: doc.value };
    return undefined;
  }
  function mapCompletionKind(kind?: number): Monaco.languages.CompletionItemKind {
    const name = kind ? LSP_COMPLETION_KIND[kind] : 'Text';
    const table = monaco.languages.CompletionItemKind as unknown as Record<string, number>;
    return (table[name] ?? table.Text) as Monaco.languages.CompletionItemKind;
  }

  function isOurModel(model: Monaco.editor.ITextModel): boolean {
    return model === editor.getModel();
  }

  disposables.push(
    monaco.languages.registerCompletionItemProvider('php', {
      triggerCharacters: ['>', ':', '$', '\\', '-', '.'],
      async provideCompletionItems(model, position) {
        if (!isOurModel(model)) return { suggestions: [] };
        let res: any;
        try {
          res = await request('textDocument/completion', {
            textDocument: { uri: documentUri },
            position: toLspPos(position)
          });
        } catch {
          return { suggestions: [] };
        }
        const items: any[] = Array.isArray(res) ? res : (res?.items ?? []);
        const word = model.getWordUntilPosition(position);
        const range: Monaco.IRange = {
          startLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endLineNumber: position.lineNumber,
          endColumn: word.endColumn
        };
        const suggestions = items.map((it: any): Monaco.languages.CompletionItem => {
          const edit = it.textEdit;
          return {
            label: it.label,
            kind: mapCompletionKind(it.kind),
            detail: it.detail,
            documentation: toMarkdown(it.documentation),
            insertText: edit?.newText ?? it.insertText ?? it.label,
            insertTextRules: it.insertTextFormat === 2
              ? monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet
              : undefined,
            range: edit?.range ? fromLspRange(edit.range) : range,
            sortText: it.sortText,
            filterText: it.filterText,
            preselect: it.preselect
          };
        });
        return { suggestions, incomplete: !!res?.isIncomplete };
      }
    })
  );

  disposables.push(
    monaco.languages.registerHoverProvider('php', {
      async provideHover(model, position) {
        if (!isOurModel(model)) return null;
        let res: any;
        try {
          res = await request('textDocument/hover', {
            textDocument: { uri: documentUri },
            position: toLspPos(position)
          });
        } catch {
          return null;
        }
        if (!res?.contents) return null;
        const raw = Array.isArray(res.contents) ? res.contents : [res.contents];
        const contents = raw.map(toMarkdown).filter(Boolean) as Monaco.IMarkdownString[];
        if (!contents.length) return null;
        return { contents, range: res.range ? fromLspRange(res.range) : undefined };
      }
    })
  );

  disposables.push(
    monaco.languages.registerSignatureHelpProvider('php', {
      signatureHelpTriggerCharacters: ['(', ','],
      async provideSignatureHelp(model, position) {
        if (!isOurModel(model)) return null;
        let res: any;
        try {
          res = await request('textDocument/signatureHelp', {
            textDocument: { uri: documentUri },
            position: toLspPos(position)
          });
        } catch {
          return null;
        }
        if (!res?.signatures?.length) return null;
        return {
          value: {
            signatures: res.signatures.map((s: any) => ({
              label: s.label,
              documentation: toMarkdown(s.documentation),
              parameters: (s.parameters ?? []).map((pa: any) => ({
                label: pa.label,
                documentation: toMarkdown(pa.documentation)
              }))
            })),
            activeSignature: res.activeSignature ?? 0,
            activeParameter: res.activeParameter ?? 0
          },
          dispose() {}
        };
      }
    })
  );

  // ---- handshake + initialize ----
  void (async () => {
    const root = await rootReady;
    if (disposed) return;
    const rootUri = 'file://' + root.split('/').map(encodeURIComponent).join('/');
    documentUri = `${rootUri}/.lerd-tinker.php`;
    try {
      await request('initialize', {
        processId: null,
        rootUri,
        workspaceFolders: [{ uri: rootUri, name: 'tinker' }],
        capabilities: {
          workspace: { workspaceFolders: true, configuration: true },
          textDocument: {
            synchronization: { dynamicRegistration: false, didSave: false },
            completion: {
              contextSupport: true,
              completionItem: { snippetSupport: true, documentationFormat: ['markdown', 'plaintext'] }
            },
            hover: { contentFormat: ['markdown', 'plaintext'] },
            signatureHelp: { signatureInformation: { documentationFormat: ['markdown', 'plaintext'] } },
            publishDiagnostics: {}
          }
        }
      });
    } catch {
      if (!disposed) onStatus?.('unavailable');
      return;
    }
    if (disposed) return;
    notify('initialized', {});
    notify('textDocument/didOpen', {
      textDocument: { uri: documentUri, languageId: 'php', version: docVersion, text: fullText() }
    });
    opened = true;
    onStatus?.('ready');
  })();

  return {
    dispose() {
      disposed = true;
      for (const d of disposables) d.dispose();
      for (const p of pending.values()) clearTimeout(p.timer);
      pending.clear();
      const model = editor.getModel();
      if (model) monaco.editor.setModelMarkers(model, 'phpantom', []);
      try {
        if (ws.readyState === WebSocket.OPEN) {
          notify('shutdown', null);
          notify('exit', null);
        }
        ws.close();
      } catch {
        /* ignore */
      }
    }
  };
}
