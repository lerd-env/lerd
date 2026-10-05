import { describe, it, expect } from 'vitest';
import { formatSql } from './sqlFormat';

describe('formatSql', () => {
  it('puts each clause on a line, the columns one per line and conditions indented', () => {
    expect(formatSql("select `id`, `name` from `users` left join `teams` on `teams`.`id` = `users`.`team_id` where `id` = 'a, b' and `deleted_at` is null order by `id` limit 5")).toBe(
      ['select', '  `id`,', '  `name`', 'from `users`', '  left join `teams` on `teams`.`id` = `users`.`team_id`', "where `id` = 'a, b'", '  and `deleted_at` is null', 'order by `id`', 'limit 5'].join('\n')
    );
  });

  it('indents a subquery a level deeper', () => {
    expect(formatSql('select * from a where id in (select count(*) from b where x in (1, 2)) and y = 1')).toBe(
      ['select *', 'from a', 'where id in (', '  select count(*)', '  from b', '  where x in (1, 2)', ')', '  and y = 1'].join('\n')
    );
  });
});
