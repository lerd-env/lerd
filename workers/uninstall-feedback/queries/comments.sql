SELECT day, reason, version, os, comment FROM answers WHERE comment != '' ORDER BY id DESC LIMIT 50;
