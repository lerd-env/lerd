import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { presets, type Preset } from './presets';
import type { Service } from './services';
import {
  adminKeyFor,
  adminServiceFor,
  suggestedPresetFor,
  suggestionFor,
  dismissedSuggestions
} from './presetSuggestions';

function svc(over: Partial<Service> & { name: string }): Service {
  return { status: 'active', site_count: 0, ...over };
}

const pgadmin: Preset = {
  name: 'pgadmin',
  description: 'Postgres admin UI',
  installed: false,
  admin_for: ['postgres', 'postgres-pgvector']
};
const phpmyadmin: Preset = {
  name: 'phpmyadmin',
  description: 'MySQL admin UI',
  installed: false,
  admin_for: ['mysql', 'mariadb']
};
const redisinsight: Preset = {
  name: 'redisinsight',
  description: 'Redis admin UI',
  installed: false,
  admin_for: ['redis', 'valkey']
};

beforeEach(() => {
  presets.set([pgadmin, phpmyadmin, redisinsight]);
  dismissedSuggestions.set([]);
});

describe('adminKeyFor', () => {
  it('uses the service name when it is its own preset', () => {
    expect(adminKeyFor(svc({ name: 'postgres' }))).toBe('postgres');
  });

  it('uses the preset for a versioned family member', () => {
    expect(adminKeyFor(svc({ name: 'postgres-17', preset: 'postgres' }))).toBe('postgres');
    expect(adminKeyFor(svc({ name: 'mariadb-11-8', preset: 'mariadb' }))).toBe('mariadb');
  });

  it('returns null for nullish input', () => {
    expect(adminKeyFor(null)).toBeNull();
    expect(adminKeyFor(undefined)).toBeNull();
  });
});

describe('suggestedPresetFor', () => {
  it('suggests pgadmin for a versioned postgres-17 service', () => {
    expect(suggestedPresetFor(svc({ name: 'postgres-17', preset: 'postgres' }))?.name).toBe(
      'pgadmin'
    );
  });

  it('still suggests pgadmin for the bare postgres service', () => {
    expect(suggestedPresetFor(svc({ name: 'postgres' }))?.name).toBe('pgadmin');
  });

  // admin_for, unlike depends_on, lets one UI claim a whole family. phpMyAdmin
  // never depends on mariadb, and RedisInsight never depends on valkey.
  it('suggests phpmyadmin for mariadb, which it administers but never depends on', () => {
    expect(suggestedPresetFor(svc({ name: 'mariadb-11-8', preset: 'mariadb' }))?.name).toBe(
      'phpmyadmin'
    );
  });

  it('suggests redisinsight for valkey', () => {
    expect(suggestedPresetFor(svc({ name: 'valkey' }))?.name).toBe('redisinsight');
  });

  it('returns null for a service nothing administers', () => {
    expect(suggestedPresetFor(svc({ name: 'mailpit' }))).toBeNull();
  });

  it('returns null once the suggestion is dismissed', () => {
    dismissedSuggestions.set(['pgadmin']);
    expect(suggestedPresetFor(svc({ name: 'postgres-17', preset: 'postgres' }))).toBeNull();
  });

  it('returns null when the admin tool is already installed', () => {
    presets.set([{ ...pgadmin, installed: true }]);
    expect(suggestedPresetFor(svc({ name: 'postgres-17', preset: 'postgres' }))).toBeNull();
  });

  it('returns null when the admin tool has an unmet dependency', () => {
    presets.set([{ ...pgadmin, missing_deps: ['postgres'] }]);
    expect(suggestedPresetFor(svc({ name: 'postgres-17', preset: 'postgres' }))).toBeNull();
  });
});

describe('adminServiceFor', () => {
  it('finds the installed admin service that administers the given service', () => {
    const installed = [svc({ name: 'redisinsight', admin_for: ['redis', 'valkey'] })];
    expect(adminServiceFor(svc({ name: 'valkey' }), installed)?.name).toBe('redisinsight');
  });

  it('resolves through the preset for a versioned family member', () => {
    const installed = [svc({ name: 'phpmyadmin', admin_for: ['mysql', 'mariadb'] })];
    const mariadb = svc({ name: 'mariadb-11-8', preset: 'mariadb' });
    expect(adminServiceFor(mariadb, installed)?.name).toBe('phpmyadmin');
  });

  it('returns null when no installed service administers it', () => {
    expect(adminServiceFor(svc({ name: 'valkey' }), [])).toBeNull();
  });
});

describe('suggestionFor', () => {
  it('reactively suggests pgadmin for postgres-17', () => {
    expect(get(suggestionFor(svc({ name: 'postgres-17', preset: 'postgres' })))?.name).toBe(
      'pgadmin'
    );
  });

  it('drops the suggestion when dismissed', () => {
    dismissedSuggestions.set(['pgadmin']);
    expect(get(suggestionFor(svc({ name: 'postgres-17', preset: 'postgres' })))).toBeNull();
  });

  it('drops the suggestion when the admin tool has an unmet dependency', () => {
    presets.set([{ ...pgadmin, missing_deps: ['postgres'] }]);
    expect(get(suggestionFor(svc({ name: 'postgres-17', preset: 'postgres' })))).toBeNull();
  });
});
