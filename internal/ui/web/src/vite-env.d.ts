/// <reference types="vite/client" />

// Monaco ships no types for the PHP grammar itself, which tinker re-registers.
declare module 'monaco-editor/languages/definitions/php/php' {
  export const language: { tokenizer: Record<string, unknown> };
}

// Internal Monarch entrypoints, used only by the grammar tests to tokenise
// the config languages through Monaco's real engine.
declare module 'monaco-editor/editor/standalone/common/monarch/monarchCompile' {
  export function compile(languageId: string, json: unknown): unknown;
}
declare module 'monaco-editor/editor/standalone/common/monarch/monarchLexer' {
  export class MonarchTokenizer {
    constructor(languageService: unknown, themeService: unknown, languageId: string, lexer: unknown, configurationService: unknown);
    getInitialState(): unknown;
    tokenize(line: string, hasEOL: boolean, state: unknown): { tokens: { offset: number; type: string }[]; endState: unknown };
  }
}
