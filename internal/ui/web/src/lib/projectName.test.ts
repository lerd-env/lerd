import { describe, it, expect } from 'vitest';
import { slugifyProjectName, finishProjectName } from './projectName';

describe('project name', () => {
  it('turns spaces and capitals into a slug as you type', () => {
    expect(slugifyProjectName('My App')).toBe('my-app');
    expect(slugifyProjectName('Shop_API  v2')).toBe('shop-api-v2');
    expect(slugifyProjectName('acme.com')).toBe('acme.com');
  });

  // Typing "my " on the way to "my-app" must leave the dash in place, or the
  // next letter lands straight after "my".
  it('keeps a trailing dash while typing', () => {
    expect(slugifyProjectName('my ')).toBe('my-');
    expect(slugifyProjectName('my-')).toBe('my-');
  });

  it('never starts with a dash or a dot', () => {
    expect(slugifyProjectName(' -.app')).toBe('app');
  });

  it('drops a trailing dash or dot once the name is submitted', () => {
    expect(finishProjectName('my-app-')).toBe('my-app');
    expect(finishProjectName('my-app.')).toBe('my-app');
  });
});
