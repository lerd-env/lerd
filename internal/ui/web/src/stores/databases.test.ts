import { describe, it, expect } from 'vitest';
import { dsnFor, type DatabaseEngine } from './databases';

function engine(connection_url?: string): DatabaseEngine {
  return {
    service: 'mysql',
    family: 'mysql',
    status: 'active',
    supports_create: true,
    supports_snapshot: true,
    databases: [],
    connection_url
  };
}

describe('dsnFor', () => {
  it('rewrites the database path for a SQL DSN', () => {
    const dsn = dsnFor(engine('mysql://root:lerd@127.0.0.1:3306/lerd'), 'shop');
    expect(dsn).toBe('mysql://root:lerd@127.0.0.1:3306/shop');
  });

  it('keeps query params when swapping the mongo database', () => {
    const dsn = dsnFor(engine('mongodb://root:lerd@127.0.0.1:27017/?authSource=admin'), 'analytics');
    expect(dsn).toBe('mongodb://root:lerd@127.0.0.1:27017/analytics?authSource=admin');
  });

  it('returns empty when the engine has no connection string', () => {
    expect(dsnFor(engine(undefined), 'shop')).toBe('');
  });
});
