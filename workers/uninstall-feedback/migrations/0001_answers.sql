-- No install id and no address: an answer cannot be tied back to anyone.
CREATE TABLE answers (
  id INTEGER PRIMARY KEY,
  day TEXT NOT NULL,
  reason TEXT NOT NULL,
  comment TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL,
  os TEXT NOT NULL
);
CREATE INDEX answers_day ON answers (day);
