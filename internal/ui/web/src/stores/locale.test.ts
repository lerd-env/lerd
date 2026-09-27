import { describe, it, expect, beforeEach } from 'vitest';
import { syncDashboardLocale } from './locale';

const cookie = () => document.cookie.split('; ').find((c) => c.startsWith('lerd_locale='));

describe('syncDashboardLocale', () => {
  beforeEach(() => {
    localStorage.clear();
    document.cookie = 'lerd_locale=; path=/; max-age=0';
  });

  // Embedded dashboards pick their language through the proxy, which can only
  // read a cookie, not the choice the language switcher keeps in storage.
  it('hands a language picked in lerd to the dashboard proxy', () => {
    localStorage.setItem('PARAGLIDE_LOCALE', 'ro');
    syncDashboardLocale();
    expect(cookie()).toBe('lerd_locale=ro');
  });

  // Without a pick lerd is already following the browser, and so are they.
  it('leaves the browser language alone when nothing was picked', () => {
    document.cookie = 'lerd_locale=de; path=/';
    syncDashboardLocale();
    expect(cookie()).toBeUndefined();
  });
});
