import { describe, it, expect } from 'vitest';
import { languageFor } from './gitDiff';

describe('languageFor', () => {
  it('picks a grammar lerd ships and falls back to plain text', () => {
    expect(languageFor('app/Models/User.php')).toBe('php');
    expect(languageFor('.env')).toBe('dotenv');
    expect(languageFor('deploy/.env.production')).toBe('dotenv');
    expect(languageFor('php/custom.ini')).toBe('ini');
    expect(languageFor('nginx/site.conf')).toBe('nginx');
    expect(languageFor('.lerd.yaml')).toBe('plaintext');
    // A folder named like a grammar says nothing about the file in it.
    expect(languageFor('.env.d/notes.txt')).toBe('plaintext');
  });
});
