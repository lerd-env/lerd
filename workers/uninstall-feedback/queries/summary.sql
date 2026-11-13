SELECT reason, count(*) AS answers FROM answers WHERE day >= date('now', '-30 days') GROUP BY reason ORDER BY answers DESC;
