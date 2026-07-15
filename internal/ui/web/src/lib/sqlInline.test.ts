import { describe, it, expect } from 'vitest';
import { inlineBindings } from './sqlInline';

describe('inlineBindings', () => {
  it('returns the SQL untouched when there are no bindings', () => {
    const sql = 'select * from users where id = ?';
    expect(inlineBindings(sql, [])).toBe(sql);
    expect(inlineBindings(sql, undefined)).toBe(sql);
  });

  it('substitutes positional placeholders in order', () => {
    expect(inlineBindings('select * from users where id = ? and team = ?', [7, 3])).toBe(
      'select * from users where id = 7 and team = 3'
    );
  });

  it('quotes strings and doubles embedded single quotes', () => {
    expect(inlineBindings('where name = ?', ["O'Brien"])).toBe("where name = 'O''Brien'");
  });

  it('renders null bindings as NULL', () => {
    expect(inlineBindings('where deleted_at = ?', [null])).toBe('where deleted_at = NULL');
  });

  it('renders booleans as 1 and 0', () => {
    expect(inlineBindings('where active = ? and banned = ?', [true, false])).toBe(
      'where active = 1 and banned = 0'
    );
  });

  it('leaves question marks inside single-quoted strings alone', () => {
    expect(inlineBindings("where label = 'why?' and id = ?", [5])).toBe(
      "where label = 'why?' and id = 5"
    );
  });

  it('leaves question marks inside double-quoted strings and backtick identifiers alone', () => {
    expect(inlineBindings('where "col?" = ? and `weird?col` = ?', [1, 2])).toBe(
      'where "col?" = 1 and `weird?col` = 2'
    );
  });

  it('handles doubled quotes inside a string literal without ending it early', () => {
    expect(inlineBindings("where note = 'it''s ok?' and id = ?", [9])).toBe(
      "where note = 'it''s ok?' and id = 9"
    );
  });

  it('handles backslash-escaped quotes inside a string literal', () => {
    expect(inlineBindings("where note = 'a\\'? b' and id = ?", [4])).toBe(
      "where note = 'a\\'? b' and id = 4"
    );
  });

  it('serializes objects and arrays as JSON string literals', () => {
    expect(inlineBindings('where meta = ?', [{ a: 1 }])).toBe('where meta = \'{"a":1}\'');
  });

  it('leaves surplus placeholders untouched when bindings run out', () => {
    expect(inlineBindings('where a = ? and b = ?', [1])).toBe('where a = 1 and b = ?');
  });
});
