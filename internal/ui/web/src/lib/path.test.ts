import { describe, it, expect } from 'vitest';
import { homeShorten } from './path';

describe('homeShorten', () => {
  const home = '/home/george';

  it('collapses the home directory itself to ~', () => {
    expect(homeShorten(home, home)).toBe('~');
  });

  it('collapses a path under home to ~/...', () => {
    expect(homeShorten('/home/george/Code/app', home)).toBe('~/Code/app');
  });

  it('leaves paths outside home untouched', () => {
    expect(homeShorten('/var/www/app', home)).toBe('/var/www/app');
  });

  it('does not collapse a sibling that only shares the prefix', () => {
    expect(homeShorten('/home/georgette/app', home)).toBe('/home/georgette/app');
  });

  it('returns the path unchanged when home is empty', () => {
    expect(homeShorten('/home/george/app', '')).toBe('/home/george/app');
  });
});
