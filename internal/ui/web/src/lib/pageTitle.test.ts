import { describe, it, expect } from 'vitest';
import { pageSubject, pageTitle } from './pageTitle';

describe('pageSubject', () => {
  it('names the open site, ignoring its sub-tab', () => {
    expect(pageSubject('sites', 'app.test/logs')).toBe('app.test');
  });

  it('names the open service by its display name', () => {
    expect(pageSubject('services', 'mysql')).toBe('MySQL');
  });

  it('is empty on a list with nothing open, and on system', () => {
    expect(pageSubject('sites', '')).toBe('');
    expect(pageSubject('system', 'nginx')).toBe('');
  });
});

describe('pageTitle', () => {
  it('reads subject, section, product', () => {
    expect(pageTitle('Sites', 'app.test')).toBe('app.test · Sites · Lerd');
  });

  it('drops an empty subject', () => {
    expect(pageTitle('System', '')).toBe('System · Lerd');
  });
});
