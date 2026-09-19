import { describe, it, expect } from 'vitest';
import { joinDashboardPath } from './dashboardPath';

describe('joinDashboardPath', () => {
  it('leaves one slash between a mount and a rooted deep link', () => {
    expect(joinDashboardPath('/_svc/mailpit/', '/view/abc')).toBe('/_svc/mailpit/view/abc');
  });

  it('adds the missing slash between a bare dashboard and a relative link', () => {
    expect(joinDashboardPath('http://localhost:8025', 'view/abc')).toBe('http://localhost:8025/view/abc');
  });

  it('keeps a query suffix against the dashboard itself', () => {
    expect(joinDashboardPath('/_svc/adminer/', '?db=shop')).toBe('/_svc/adminer/?db=shop');
  });

  it('returns the dashboard when there is nothing to append', () => {
    expect(joinDashboardPath('/_svc/rustfs/')).toBe('/_svc/rustfs/');
  });

  it('appends a relative link to a mount that already ends in a slash', () => {
    expect(joinDashboardPath('/rustfs/console/', 'browser/?bucket=lerd')).toBe(
      '/rustfs/console/browser/?bucket=lerd'
    );
  });
});
