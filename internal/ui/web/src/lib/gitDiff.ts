import { apiJson } from './api';

export type GitDiffSection = 'staged' | 'unstaged' | 'untracked';

export interface GitChangedFile {
  path: string;
  // git's letter: A, M, D, T, U, or ? for untracked.
  status: string;
}

export type GitChanges = Record<GitDiffSection, GitChangedFile[]>;

export interface GitVersions {
  original: string;
  modified: string;
  binary: boolean;
  too_large: boolean;
}

export const SECTIONS: GitDiffSection[] = ['staged', 'unstaged', 'untracked'];

function query(domain: string, path: string): string {
  return `domain=${encodeURIComponent(domain)}&path=${encodeURIComponent(path)}`;
}

export function loadGitChanges(domain: string, path: string): Promise<GitChanges> {
  return apiJson<GitChanges>(`/api/sites/git-diff?${query(domain, path)}`);
}

export function loadGitVersions(domain: string, path: string, section: GitDiffSection, file: string): Promise<GitVersions> {
  return apiJson<GitVersions>(
    `/api/sites/git-diff?${query(domain, path)}&section=${section}&file=${encodeURIComponent(file)}&content=1`
  );
}

// languageFor picks one of the grammars lerd's Monaco build carries; anything
// else is shown as plain text.
export function languageFor(file: string): string {
  const name = file.split('/').pop() ?? file;
  if (name.endsWith('.php')) return 'php';
  if (name === '.env' || name.startsWith('.env.')) return 'dotenv';
  if (name.endsWith('.ini')) return 'ini';
  if (name.endsWith('.conf')) return 'nginx';
  return 'plaintext';
}
