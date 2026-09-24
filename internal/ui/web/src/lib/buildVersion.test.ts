import { describe, it, expect } from 'vitest';
import { parseBuildVersion } from './buildVersion';

describe('parseBuildVersion', () => {
  it('reads a tagged release as a release with no commit', () => {
    expect(parseBuildVersion('1.35.0')).toEqual({ channel: 'release', base: '1.35.0', commit: null });
  });

  it('reads a beta tag as beta', () => {
    expect(parseBuildVersion('1.35.0-beta.4')).toEqual({ channel: 'beta', base: '1.35.0', commit: null });
  });

  // git describe past a tag: <tag>-<commits since>-g<hash>[-dirty]
  it('pulls the commit out of a dev build', () => {
    expect(parseBuildVersion('1.35.0-48-ga75062ab-dirty')).toEqual({ channel: 'dev', base: '1.35.0', commit: 'a75062ab' });
    expect(parseBuildVersion('1.35.0-48-ga75062ab')).toEqual({ channel: 'dev', base: '1.35.0', commit: 'a75062ab' });
  });

  it('treats a build on top of a beta as dev', () => {
    expect(parseBuildVersion('1.35.0-beta.4-3-gdeadbee')).toEqual({ channel: 'dev', base: '1.35.0', commit: 'deadbee' });
  });

  // --always falls back to the bare hash when no tag is reachable.
  it('reads a bare hash as dev', () => {
    expect(parseBuildVersion('a75062ab-dirty')).toEqual({ channel: 'dev', base: null, commit: 'a75062ab' });
  });

  it('marks uncommitted changes on a tag as dev', () => {
    expect(parseBuildVersion('1.35.0-dirty')).toEqual({ channel: 'dev', base: '1.35.0', commit: null });
  });
});
