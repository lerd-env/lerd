#ifdef HAVE_CONFIG_H
#include "config.h"
#endif

#include "php.h"
#include "php_ini.h"
#include "ext/standard/info.h"
#include "php_lerd_devtools.h"

ZEND_DECLARE_MODULE_GLOBALS(lerd_devtools)

PHP_INI_BEGIN()
	STD_PHP_INI_ENTRY("lerd.devtools_host", "", PHP_INI_ALL, OnUpdateString, host, zend_lerd_devtools_globals, lerd_devtools_globals)
	STD_PHP_INI_ENTRY("lerd.devtools_kinds", "query", PHP_INI_ALL, OnUpdateString, kinds, zend_lerd_devtools_globals, lerd_devtools_globals)
	STD_PHP_INI_ENTRY("lerd.devtools_flag", "/usr/local/etc/lerd/devtools.flag", PHP_INI_ALL, OnUpdateString, flag, zend_lerd_devtools_globals, lerd_devtools_globals)
PHP_INI_END()

/* The capture path only exists where the zend_observer API does (PHP 8.0+).
 * On the legacy tier the module still loads so the image build succeeds; it
 * simply observes nothing. */
#if PHP_VERSION_ID >= 80000
#define LERD_OBSERVE 1
#endif

#ifdef LERD_OBSERVE
#include "zend_observer.h"
#include "zend_smart_str.h"
#include "zend_execute.h"
#include "zend_constants.h"
#include "SAPI.h"
#include <time.h>
#include <unistd.h>
#include <string.h>
#include <stdlib.h>
#include <stdio.h>
#include <fcntl.h>
#include <poll.h>
#include <errno.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <netinet/in.h>
#include <netdb.h>
#include <arpa/inet.h>

/* ── transport ──────────────────────────────────────────────────────────── */

/* lerd_send writes one NDJSON line to the configured socket and closes it.
 * Fire-and-forget with a 50ms connect budget; any error is swallowed so a
 * down or slow lerd-ui never disturbs the request. */
static void lerd_send(const char *line, size_t len)
{
	const char *host = LERD_G(host);
	if (!host || !host[0]) {
		return;
	}

	int fd = -1;
	if (strncmp(host, "unix://", 7) == 0) {
		const char *path = host + 7;
		struct sockaddr_un sa;
		if (strlen(path) >= sizeof(sa.sun_path)) {
			return;
		}
		fd = socket(AF_UNIX, SOCK_STREAM, 0);
		if (fd < 0) {
			return;
		}
		memset(&sa, 0, sizeof(sa));
		sa.sun_family = AF_UNIX;
		strncpy(sa.sun_path, path, sizeof(sa.sun_path) - 1);
		fcntl(fd, F_SETFL, O_NONBLOCK);
		if (connect(fd, (struct sockaddr *)&sa, sizeof(sa)) != 0 && errno != EINPROGRESS) {
			close(fd);
			return;
		}
	} else {
		/* tcp://host:port (used on macOS where the unix socket can't cross
		 * the podman-machine boundary). */
		const char *hp = host;
		if (strncmp(host, "tcp://", 6) == 0) {
			hp = host + 6;
		}
		char hbuf[256];
		strncpy(hbuf, hp, sizeof(hbuf) - 1);
		hbuf[sizeof(hbuf) - 1] = '\0';
		char *colon = strrchr(hbuf, ':');
		if (!colon) {
			return;
		}
		*colon = '\0';
		int port = atoi(colon + 1);
		if (port <= 0) {
			return;
		}
		struct addrinfo hints, *res = NULL;
		memset(&hints, 0, sizeof(hints));
		hints.ai_family = AF_UNSPEC;
		hints.ai_socktype = SOCK_STREAM;
		if (getaddrinfo(hbuf, colon + 1, &hints, &res) != 0 || !res) {
			return;
		}
		fd = socket(res->ai_family, res->ai_socktype, res->ai_protocol);
		if (fd < 0) {
			freeaddrinfo(res);
			return;
		}
		fcntl(fd, F_SETFL, O_NONBLOCK);
		if (connect(fd, res->ai_addr, res->ai_addrlen) != 0 && errno != EINPROGRESS) {
			freeaddrinfo(res);
			close(fd);
			return;
		}
		freeaddrinfo(res);
	}

	struct pollfd pfd;
	pfd.fd = fd;
	pfd.events = POLLOUT;
	if (poll(&pfd, 1, 50) <= 0 || !(pfd.revents & POLLOUT)) {
		close(fd);
		return;
	}

	size_t off = 0;
	while (off < len) {
		ssize_t n = write(fd, line + off, len - off);
		if (n <= 0) {
			break;
		}
		off += (size_t)n;
	}
	if (off >= len) {
		(void)write(fd, "\n", 1);
	}
	close(fd);
}

/* ── helpers ────────────────────────────────────────────────────────────── */

static void append_json_string(smart_str *buf, const char *s, size_t len)
{
	smart_str_appendc(buf, '"');
	for (size_t i = 0; i < len; i++) {
		unsigned char c = (unsigned char)s[i];
		switch (c) {
			case '"':  smart_str_appendl(buf, "\\\"", 2); break;
			case '\\': smart_str_appendl(buf, "\\\\", 2); break;
			case '\n': smart_str_appendl(buf, "\\n", 2); break;
			case '\r': smart_str_appendl(buf, "\\r", 2); break;
			case '\t': smart_str_appendl(buf, "\\t", 2); break;
			default:
				if (c < 0x20) {
					char esc[7];
					snprintf(esc, sizeof(esc), "\\u%04x", c);
					smart_str_appendl(buf, esc, 6);
				} else {
					smart_str_appendc(buf, (char)c);
				}
		}
	}
	smart_str_appendc(buf, '"');
}

static void append_json_kv_str(smart_str *buf, const char *key, const char *val, size_t vlen)
{
	smart_str_appendc(buf, '"');
	smart_str_appends(buf, key);
	smart_str_appendl(buf, "\":", 2);
	append_json_string(buf, val, vlen);
}

/* server_var returns $_SERVER[name] (fastcgi_param under FPM), falling back to
 * the process environment for CLI/tinker/queue workers. */
static const char *server_var(const char *name, size_t len, smart_str *scratch)
{
	zval *arr, *v;
	if (zend_is_auto_global_str("_SERVER", sizeof("_SERVER") - 1)) {
		arr = &PG(http_globals)[TRACK_VARS_SERVER];
		if (Z_TYPE_P(arr) == IS_ARRAY) {
			v = zend_hash_str_find(Z_ARRVAL_P(arr), name, len);
			if (v && Z_TYPE_P(v) == IS_STRING && Z_STRLEN_P(v) > 0) {
				smart_str_appendl(scratch, Z_STRVAL_P(v), Z_STRLEN_P(v));
				smart_str_0(scratch);
				return ZSTR_VAL(scratch->s);
			}
		}
	}
	return getenv(name);
}

static void append_iso_ts(smart_str *buf)
{
	struct timespec ts;
	clock_gettime(CLOCK_REALTIME, &ts);
	struct tm tmv;
	gmtime_r(&ts.tv_sec, &tmv);
	char out[40];
	int n = (int)strftime(out, sizeof(out), "%Y-%m-%dT%H:%M:%S", &tmv);
	snprintf(out + n, sizeof(out) - n, ".%03dZ", (int)(ts.tv_nsec / 1000000));
	smart_str_appends(buf, out);
}

/* Long-running queue/scheduler workers poll the DB constantly; their queries
 * are noise that floods the buffer and aren't tied to a request worth
 * inspecting. lerd_is_worker is set once at MINIT from the process command
 * line so the observer can skip them. Detected here rather than via a
 * lerd-set env var so it works without re-launching workers with new flags. */
static zend_bool lerd_is_worker = 0;

/* lerd_laravel is set once the Laravel adapter has been loaded at boot. While
 * it's set, the engine-level PDO observer stops emitting query events: the
 * adapter captures them from QueryExecuted with richer data (real bindings,
 * connection name, per-job grouping), so we'd otherwise double-count. */
static zend_bool lerd_laravel = 0;

/* cmdline_is_worker reports whether process `pid`'s command line names a
 * long-running queue/scheduler daemon. */
static int cmdline_is_worker(int pid)
{
	char path[64];
	snprintf(path, sizeof(path), "/proc/%d/cmdline", pid);
	int fd = open(path, O_RDONLY);
	if (fd < 0) {
		return 0;
	}
	char buf[4096];
	ssize_t n = read(fd, buf, sizeof(buf) - 1);
	close(fd);
	if (n <= 0) {
		return 0;
	}
	for (ssize_t i = 0; i < n; i++) {
		if (buf[i] == '\0') {
			buf[i] = ' ';
		}
	}
	buf[n] = '\0';
	static const char *markers[] = {
		"queue:work", "queue:listen", "horizon", "schedule:work",
		"schedule:run", "messenger:consume", NULL
	};
	for (int i = 0; markers[i]; i++) {
		if (strstr(buf, markers[i])) {
			return 1;
		}
	}
	return 0;
}

/* read_ppid returns the parent pid from /proc/<pid>/stat. The comm field is
 * parenthesised and may contain spaces, so we parse after the last ')'. */
static int read_ppid(int pid)
{
	char path[64];
	snprintf(path, sizeof(path), "/proc/%d/stat", pid);
	int fd = open(path, O_RDONLY);
	if (fd < 0) {
		return 0;
	}
	char buf[512];
	ssize_t n = read(fd, buf, sizeof(buf) - 1);
	close(fd);
	if (n <= 0) {
		return 0;
	}
	buf[n] = '\0';
	char *rp = strrchr(buf, ')');
	if (!rp) {
		return 0;
	}
	int ppid = 0;
	if (sscanf(rp + 1, " %*c %d", &ppid) != 1) {
		return 0;
	}
	return ppid;
}

/* lerd_worker_cmd holds this process's artisan command (e.g. "queue:work",
 * "scrape:rtb-data") so worker queries can be labelled and filtered by it. */
static char lerd_worker_cmd[64] = "";
static void extract_self_cmd(void)
{
	int fd = open("/proc/self/cmdline", O_RDONLY);
	if (fd < 0) {
		return;
	}
	char buf[4096];
	ssize_t n = read(fd, buf, sizeof(buf) - 1);
	close(fd);
	if (n <= 0) {
		return;
	}
	buf[n] = '\0';
	/* Tokens are NUL-separated, so each is already a C string. Find the
	 * "artisan" token; the next token is the command name. */
	ssize_t k = 0;
	while (k < n) {
		char *tok = &buf[k];
		size_t len = strlen(tok);
		const char *base = strrchr(tok, '/');
		base = base ? base + 1 : tok;
		if (strcmp(base, "artisan") == 0) {
			ssize_t next = k + (ssize_t)len + 1;
			if (next < n && buf[next] != '-') {
				strncpy(lerd_worker_cmd, &buf[next], sizeof(lerd_worker_cmd) - 1);
				lerd_worker_cmd[sizeof(lerd_worker_cmd) - 1] = '\0';
			}
			return;
		}
		k += (ssize_t)len + 1;
	}
}

/* A scheduled command runs as a child of `schedule:run`, and a queued job can
 * run as a child of the worker, so the worker marker may only appear on an
 * ancestor's command line, not this process's. Walk a few levels up. */
static void detect_worker(void)
{
	int pid = getpid();
	for (int depth = 0; depth < 6 && pid > 1; depth++) {
		if (cmdline_is_worker(pid)) {
			lerd_is_worker = 1;
			extract_self_cmd();
			if (!lerd_worker_cmd[0]) {
				strcpy(lerd_worker_cmd, "worker");
			}
			return;
		}
		pid = read_ppid(pid);
	}
}

/* PHPUnit's own bootstrap defines PHPUNIT_COMPOSER_INSTALL on every run, and
 * Pest runs on PHPUnit, so this tags test traffic without naming a framework.
 * Sticky: the constant lands after RINIT and a run never leaves test mode. */
static zend_bool lerd_in_test(void)
{
	if (LERD_G(is_test)) {
		return 1;
	}
	if (zend_hash_str_exists(EG(zend_constants), "PHPUNIT_COMPOSER_INSTALL", sizeof("PHPUNIT_COMPOSER_INSTALL") - 1)
		|| zend_hash_str_exists(EG(class_table), "phpunit\\framework\\testcase", sizeof("phpunit\\framework\\testcase") - 1)) {
		LERD_G(is_test) = 1;
	}
	return LERD_G(is_test);
}

/* event ids increase with wall-clock so the ring's SinceID cursor stays sane.
 * Not a real ULID; just monotonic-enough hex plus a per-process counter. */
static unsigned long lerd_seq = 0;
static void append_event_id(smart_str *buf)
{
	struct timespec ts;
	clock_gettime(CLOCK_REALTIME, &ts);
	unsigned long long us = (unsigned long long)ts.tv_sec * 1000000ULL + (unsigned long long)(ts.tv_nsec / 1000);
	char out[40];
	snprintf(out, sizeof(out), "%015llx%08lx", us, (lerd_seq++ & 0xffffffffUL));
	smart_str_appends(buf, out);
}

/* append_bindings serialises a flat array of scalars (PDOStatement::execute's
 * input-parameter array) as a JSON array. Non-scalars become null. */
static void append_bindings(smart_str *buf, zval *arr)
{
	smart_str_appendl(buf, "\"bindings\":[", 12);
	zend_bool first = 1;
	zval *v;
	ZEND_HASH_FOREACH_VAL(Z_ARRVAL_P(arr), v) {
		if (!first) {
			smart_str_appendc(buf, ',');
		}
		first = 0;
		ZVAL_DEREF(v);
		switch (Z_TYPE_P(v)) {
			case IS_STRING:
				append_json_string(buf, Z_STRVAL_P(v), Z_STRLEN_P(v));
				break;
			case IS_LONG: {
				char nb[32];
				snprintf(nb, sizeof(nb), ZEND_LONG_FMT, Z_LVAL_P(v));
				smart_str_appends(buf, nb);
				break;
			}
			case IS_DOUBLE: {
				char nb[40];
				snprintf(nb, sizeof(nb), "%.10g", Z_DVAL_P(v));
				smart_str_appends(buf, nb);
				break;
			}
			case IS_TRUE:  smart_str_appendl(buf, "true", 4); break;
			case IS_FALSE: smart_str_appendl(buf, "false", 5); break;
			default:       smart_str_appendl(buf, "null", 4); break;
		}
	} ZEND_HASH_FOREACH_END();
	smart_str_appendc(buf, ']');
}

/* ── observer ───────────────────────────────────────────────────────────── */

static void tstack_push(double v)
{
	if (LERD_G(tstack_len) == LERD_G(tstack_cap)) {
		int cap = LERD_G(tstack_cap) ? LERD_G(tstack_cap) * 2 : 16;
		double *p = realloc(LERD_G(tstack), (size_t)cap * sizeof(double));
		if (!p) {
			return;
		}
		LERD_G(tstack) = p;
		LERD_G(tstack_cap) = cap;
	}
	LERD_G(tstack)[LERD_G(tstack_len)++] = v;
}

static double tstack_pop(void)
{
	if (LERD_G(tstack_len) <= 0) {
		return -1.0;
	}
	return LERD_G(tstack)[--LERD_G(tstack_len)];
}

static double now_ms(void)
{
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return (double)ts.tv_sec * 1000.0 + (double)ts.tv_nsec / 1e6;
}

static void lerd_obs_begin(zend_execute_data *execute_data)
{
	tstack_push(now_ms());
}

static void lerd_obs_end(zend_execute_data *execute_data, zval *retval)
{
	double start = tstack_pop();

	zend_function *fn = execute_data->func;
	if (!fn || !fn->common.function_name) {
		return;
	}
	const char *fname = ZSTR_VAL(fn->common.function_name);
	zend_bool is_stmt_execute = fn->common.scope &&
		zend_string_equals_literal_ci(fn->common.scope->name, "PDOStatement") &&
		strcasecmp(fname, "execute") == 0;

	/* Pull (and clear) any bindValue/bindParam values buffered for this
	 * statement, so the buffer never leaks across the gate checks below. */
	zval bound;
	ZVAL_UNDEF(&bound);
	if (is_stmt_execute && LERD_G(bind_buf_init) && Z_TYPE(execute_data->This) == IS_OBJECT) {
		zend_ulong h = (zend_ulong) Z_OBJ_HANDLE(execute_data->This);
		zval *b = zend_hash_index_find(&LERD_G(bind_buf), h);
		if (b) {
			ZVAL_COPY(&bound, b);
			zend_hash_index_del(&LERD_G(bind_buf), h);
		}
	}

	if (!LERD_G(active) || !LERD_G(want_query) || start < 0 ||
		lerd_laravel || (lerd_is_worker && !LERD_G(capture_workers))) {
		zval_ptr_dtor(&bound);
		return;
	}

	/* Resolve the SQL text. */
	const char *sql = NULL;
	size_t sql_len = 0;
	zval rv;
	ZVAL_UNDEF(&rv);
	if (is_stmt_execute) {
		if (Z_TYPE(execute_data->This) != IS_OBJECT) {
			return;
		}
		zend_object *obj = Z_OBJ(execute_data->This);
		zval *qs = zend_read_property(obj->ce, obj, "queryString", sizeof("queryString") - 1, 1, &rv);
		if (qs && Z_TYPE_P(qs) == IS_STRING) {
			sql = Z_STRVAL_P(qs);
			sql_len = Z_STRLEN_P(qs);
		}
	} else {
		/* PDO::query / PDO::exec — SQL is the first argument. */
		if (ZEND_CALL_NUM_ARGS(execute_data) >= 1) {
			zval *a = ZEND_CALL_ARG(execute_data, 1);
			if (a && Z_TYPE_P(a) == IS_STRING) {
				sql = Z_STRVAL_P(a);
				sql_len = Z_STRLEN_P(a);
			}
		}
	}
	if (!sql || sql_len == 0) {
		return;
	}

	double elapsed = now_ms() - start;

	/* Caller file:line. The immediate caller is almost always the framework's
	 * database layer (vendor/…/Connection.php), which is useless — we want the
	 * application frame that triggered the query. Walk up and prefer the first
	 * frame outside /vendor/; fall back to the first userland frame if every
	 * frame is vendor (e.g. a query during framework boot). */
	const char *file = NULL;
	size_t file_len = 0;
	int line = 0;
	const char *fb_file = NULL;
	size_t fb_len = 0;
	int fb_line = 0;
	/* Build the full backtrace too: in vendor-heavy apps (Filament/Nova admin
	 * panels, package controllers) the query has no application frame at all
	 * except index.php, so a single "src" line can't point at the real origin.
	 * The trace lets the user scan the whole chain in the expanded row. */
	smart_str trace = {0};
	smart_str_appendc(&trace, '[');
	zend_bool tfirst = 1;
	int tframes = 0;
	zend_execute_data *caller = execute_data->prev_execute_data;
	while (caller && tframes < 40) {
		if (caller->func && ZEND_USER_CODE(caller->func->common.type) &&
			caller->func->op_array.filename) {
			const char *cf = ZSTR_VAL(caller->func->op_array.filename);
			size_t cl = ZSTR_LEN(caller->func->op_array.filename);
			int cline = caller->opline ? (int)caller->opline->lineno : 0;
			if (!fb_file) {
				fb_file = cf;
				fb_len = cl;
				fb_line = cline;
			}
			if (!file && !strstr(cf, "/vendor/")) {
				file = cf;
				file_len = cl;
				line = cline;
			}
			if (!tfirst) {
				smart_str_appendc(&trace, ',');
			}
			tfirst = 0;
			smart_str_appendl(&trace, "{\"file\":", 8);
			append_json_string(&trace, cf, cl);
			smart_str_appendl(&trace, ",\"line\":", 8);
			{
				char lb[16];
				snprintf(lb, sizeof(lb), "%d", cline);
				smart_str_appends(&trace, lb);
			}
			smart_str_appendl(&trace, ",\"func\":", 8);
			{
				smart_str fnb = {0};
				if (caller->func->common.scope && caller->func->common.scope->name) {
					smart_str_appendl(&fnb, ZSTR_VAL(caller->func->common.scope->name), ZSTR_LEN(caller->func->common.scope->name));
					smart_str_appendl(&fnb, "::", 2);
				}
				if (caller->func->common.function_name) {
					smart_str_appendl(&fnb, ZSTR_VAL(caller->func->common.function_name), ZSTR_LEN(caller->func->common.function_name));
				} else {
					smart_str_appendl(&fnb, "{main}", 6);
				}
				smart_str_0(&fnb);
				append_json_string(&trace, ZSTR_VAL(fnb.s), ZSTR_LEN(fnb.s));
				smart_str_free(&fnb);
			}
			smart_str_appendc(&trace, '}');
			tframes++;
		}
		caller = caller->prev_execute_data;
	}
	smart_str_appendc(&trace, ']');
	smart_str_0(&trace);
	if (!file) {
		file = fb_file;
		file_len = fb_len;
		line = fb_line;
	}

	const char *sapi = sapi_module.name ? sapi_module.name : "";
	zend_bool is_cli = strncmp(sapi, "cli", 3) == 0;

	smart_str sctx = {0};
	const char *site = server_var("LERD_SITE", sizeof("LERD_SITE") - 1, &sctx);
	smart_str sbranch = {0};
	const char *branch = server_var("LERD_BRANCH", sizeof("LERD_BRANCH") - 1, &sbranch);
	/* domain + request let the dashboard draw per-request boundaries (the same
	 * grouping dumps use). Without them, every query from a site collapses into
	 * one group and duplicate/N+1 counts bleed across separate requests. */
	smart_str shost = {0}, smethod = {0}, suri = {0};
	const char *host = is_cli ? NULL : server_var("HTTP_HOST", sizeof("HTTP_HOST") - 1, &shost);
	const char *method = is_cli ? NULL : server_var("REQUEST_METHOD", sizeof("REQUEST_METHOD") - 1, &smethod);
	const char *uri = is_cli ? NULL : server_var("REQUEST_URI", sizeof("REQUEST_URI") - 1, &suri);

	smart_str buf = {0};
	smart_str_appendl(&buf, "{\"v\":1,\"id\":", 12);
	smart_str_appendc(&buf, '"');
	append_event_id(&buf);
	smart_str_appendc(&buf, '"');
	smart_str_appendl(&buf, ",\"ts\":", 6);
	smart_str_appendc(&buf, '"');
	append_iso_ts(&buf);
	smart_str_appendc(&buf, '"');
	smart_str_appendl(&buf, ",\"kind\":\"query\",\"ctx\":{\"type\":", 30);
	smart_str_appends(&buf, is_cli ? "\"cli\"" : "\"fpm\"");
	if (site && site[0]) {
		smart_str_appendc(&buf, ',');
		append_json_kv_str(&buf, "site", site, strlen(site));
	}
	if (branch && branch[0]) {
		smart_str_appendc(&buf, ',');
		append_json_kv_str(&buf, "branch", branch, strlen(branch));
	}
	if (host && host[0]) {
		smart_str_appendc(&buf, ',');
		append_json_kv_str(&buf, "domain", host, strlen(host));
	}
	if (method && method[0]) {
		smart_str req = {0};
		smart_str_appends(&req, method);
		smart_str_appendc(&req, ' ');
		if (uri) {
			smart_str_appends(&req, uri);
		}
		smart_str_0(&req);
		smart_str_appendl(&buf, ",\"request\":", 11);
		append_json_string(&buf, ZSTR_VAL(req.s), ZSTR_LEN(req.s));
		smart_str_free(&req);
	}
	{
		char pidbuf[24];
		snprintf(pidbuf, sizeof(pidbuf), "%ld", (long)getpid());
		smart_str_appendl(&buf, ",\"pid\":", 7);
		smart_str_appends(&buf, pidbuf);
	}
	if (LERD_G(rid)[0]) {
		smart_str_appendc(&buf, ',');
		append_json_kv_str(&buf, "rid", LERD_G(rid), strlen(LERD_G(rid)));
	}
	if (lerd_is_worker && lerd_worker_cmd[0]) {
		smart_str_appendc(&buf, ',');
		append_json_kv_str(&buf, "worker", lerd_worker_cmd, strlen(lerd_worker_cmd));
	}
	if (lerd_in_test()) {
		smart_str_appendl(&buf, ",\"test\":true", 12);
	}
	smart_str_appendl(&buf, "},\"src\":{", 9);
	smart_str_appendl(&buf, "\"file\":", 7);
	append_json_string(&buf, file ? file : "", file_len);
	smart_str_appendl(&buf, ",\"line\":", 8);
	{
		char lb[16];
		snprintf(lb, sizeof(lb), "%d", line);
		smart_str_appends(&buf, lb);
	}
	smart_str_appendl(&buf, "},\"data\":{", 10);
	append_json_kv_str(&buf, "sql", sql, sql_len);
	smart_str_appendl(&buf, ",\"time_ms\":", 11);
	{
		char tb[40];
		snprintf(tb, sizeof(tb), "%.3f", elapsed < 0 ? 0.0 : elapsed);
		smart_str_appends(&buf, tb);
	}
	/* Bindings: prefer the array passed to execute([...]); otherwise fall back
	 * to the values buffered from bindValue/bindParam (the common path — that's
	 * how Doctrine and Laravel bind, which is why bindings used to be null). */
	zval *explicit_params = (is_stmt_execute && ZEND_CALL_NUM_ARGS(execute_data) >= 1)
		? ZEND_CALL_ARG(execute_data, 1) : NULL;
	if (explicit_params && Z_TYPE_P(explicit_params) == IS_ARRAY && zend_hash_num_elements(Z_ARRVAL_P(explicit_params)) > 0) {
		smart_str_appendc(&buf, ',');
		append_bindings(&buf, explicit_params);
	} else if (Z_TYPE(bound) == IS_ARRAY && zend_hash_num_elements(Z_ARRVAL(bound)) > 0) {
		smart_str_appendc(&buf, ',');
		append_bindings(&buf, &bound);
	}
	if (trace.s && ZSTR_LEN(trace.s) > 2) {
		smart_str_appendl(&buf, ",\"trace\":", 9);
		smart_str_appendl(&buf, ZSTR_VAL(trace.s), ZSTR_LEN(trace.s));
	}
	smart_str_appendl(&buf, "}}", 2);
	smart_str_0(&buf);

	lerd_send(ZSTR_VAL(buf.s), ZSTR_LEN(buf.s));

	smart_str_free(&buf);
	smart_str_free(&trace);
	smart_str_free(&sctx);
	smart_str_free(&sbranch);
	smart_str_free(&shost);
	smart_str_free(&smethod);
	smart_str_free(&suri);
	zval_ptr_dtor(&bound);
}

/* lerd_bind_end buffers PDOStatement::bindValue/bindParam values per statement
 * so the agnostic query path can show real bindings (frameworks bind this way,
 * not via execute([...])). Only buffers when the agnostic path is capturing. */
static void lerd_bind_end(zend_execute_data *execute_data, zval *retval)
{
	if (!LERD_G(active) || !LERD_G(want_query) || lerd_laravel) {
		return;
	}
	if (Z_TYPE(execute_data->This) != IS_OBJECT || ZEND_CALL_NUM_ARGS(execute_data) < 2) {
		return;
	}
	zval *value = ZEND_CALL_ARG(execute_data, 2);
	if (!value) {
		return;
	}
	if (Z_TYPE_P(value) == IS_REFERENCE) {
		value = Z_REFVAL_P(value);
	}
	if (!LERD_G(bind_buf_init)) {
		zend_hash_init(&LERD_G(bind_buf), 8, NULL, ZVAL_PTR_DTOR, 0);
		LERD_G(bind_buf_init) = 1;
	}
	zend_ulong h = (zend_ulong) Z_OBJ_HANDLE(execute_data->This);
	zval *arr = zend_hash_index_find(&LERD_G(bind_buf), h);
	if (!arr) {
		zval newarr;
		array_init(&newarr);
		arr = zend_hash_index_add(&LERD_G(bind_buf), h, &newarr);
	}
	zval copy;
	ZVAL_COPY(&copy, value);
	add_next_index_zval(arr, &copy);
}

/* lerd_boot_end fires when Illuminate\Foundation\Application::boot() returns,
 * i.e. the framework is up and app() is usable. It loads the Laravel adapter
 * once, which registers the QueryExecuted/job/etc listeners. Done via eval so
 * the C side needs no PHP function table; @include keeps a missing file from
 * fataling. */
static void lerd_boot_end(zend_execute_data *execute_data, zval *retval)
{
	(void)execute_data;
	(void)retval;
	if (lerd_laravel || !LERD_G(active) || !LERD_G(want_query)) {
		return;
	}
	lerd_laravel = 1; /* set first: stops the PDO observer double-capturing */
	zend_eval_string(
		"if (function_exists('app') && !defined('LERD_LARAVEL_ADAPTER')) {"
		" define('LERD_LARAVEL_ADAPTER', 1);"
		" @include '/usr/local/etc/lerd/laravel-adapter.php';"
		"}",
		NULL, "lerd-laravel-adapter");
}

/* The agnostic collector is a framework-neutral PHP file that extracts and
 * emits events for shared libraries (mail today). Loaded lazily on first need
 * so non-framework apps pay nothing until they actually send mail etc. */
static void lerd_ensure_collector(void)
{
	if (LERD_G(collector_loaded)) {
		return;
	}
	LERD_G(collector_loaded) = 1;
	zend_eval_string(
		"if (!function_exists('Lerd\\\\Collector\\\\emit')) {"
		" @include '/usr/local/etc/lerd/devtools-collector.php';"
		"}",
		NULL, "lerd-collector-load");
}

/* lerd_mail_end captures one outgoing mail. Laravel claims mail via its own
 * adapter, so we stand down there; everyone else (Symfony, raw PHP on Symfony
 * Mailer) gets it here. Extraction is done in PHP by the collector. */
static void lerd_mail_end(zend_execute_data *execute_data, zval *retval)
{
	(void)retval;
	if (!LERD_G(active) || lerd_laravel || (lerd_is_worker && !LERD_G(capture_workers))) {
		return;
	}
	if (ZEND_CALL_NUM_ARGS(execute_data) < 1) {
		return;
	}
	zval *msg = ZEND_CALL_ARG(execute_data, 1);
	if (!msg || Z_TYPE_P(msg) != IS_OBJECT) {
		return;
	}
	lerd_ensure_collector();
	zval fname, rv, args[1];
	ZVAL_STRINGL(&fname, "Lerd\\Collector\\mail", sizeof("Lerd\\Collector\\mail") - 1);
	ZVAL_COPY(&args[0], msg);
	if (call_user_function(NULL, NULL, &fname, &rv, 1, args) == SUCCESS) {
		zval_ptr_dtor(&rv);
	}
	zval_ptr_dtor(&fname);
	zval_ptr_dtor(&args[0]);
}

/* lerd_view_end captures one Twig render. Twig is the de-facto Symfony view
 * layer, so this single seam gives Symfony (and any Twig app) a Views tab the
 * same way the Laravel adapter does for Blade. Laravel claims views itself, so
 * we stand down there. Extraction (name, source path, keys) is in the
 * collector; we just hand over the environment, template name and context. */
static void lerd_view_end(zend_execute_data *execute_data, zval *retval)
{
	(void)retval;
	if (!LERD_G(active) || lerd_laravel || (lerd_is_worker && !LERD_G(capture_workers))) {
		return;
	}
	if (ZEND_CALL_NUM_ARGS(execute_data) < 1 || Z_TYPE(execute_data->This) != IS_OBJECT) {
		return;
	}
	zval *name = ZEND_CALL_ARG(execute_data, 1);
	zval *ctx = ZEND_CALL_NUM_ARGS(execute_data) >= 2 ? ZEND_CALL_ARG(execute_data, 2) : NULL;
	if (!name) {
		return;
	}
	lerd_ensure_collector();
	zval fname, rv, args[3];
	ZVAL_STRINGL(&fname, "Lerd\\Collector\\view", sizeof("Lerd\\Collector\\view") - 1);
	ZVAL_COPY(&args[0], &execute_data->This);
	ZVAL_COPY(&args[1], name);
	if (ctx) {
		ZVAL_COPY(&args[2], ctx);
	} else {
		ZVAL_NULL(&args[2]);
	}
	if (call_user_function(NULL, NULL, &fname, &rv, 3, args) == SUCCESS) {
		zval_ptr_dtor(&rv);
	}
	zval_ptr_dtor(&fname);
	zval_ptr_dtor(&args[0]);
	zval_ptr_dtor(&args[1]);
	zval_ptr_dtor(&args[2]);
}

/* lerd_event_end captures one Symfony event dispatch. The dispatcher is the
 * de-facto Symfony event bus; Laravel claims events via its own adapter, so we
 * stand down there. The collector decides which events are app-level noise. */
static void lerd_event_end(zend_execute_data *execute_data, zval *retval)
{
	if (!LERD_G(active) || lerd_laravel || (lerd_is_worker && !LERD_G(capture_workers))) {
		return;
	}
	/* dispatch() ends with `return $event;`, so the engine moves the $event CV
	 * into the return value as a last-use optimisation and arg1 reads back as
	 * NULL in the end observer. The returned value is that same event object,
	 * so take it from retval and only fall back to arg1. */
	zval *event = (retval && Z_TYPE_P(retval) == IS_OBJECT) ? retval : NULL;
	if (!event && ZEND_CALL_NUM_ARGS(execute_data) >= 1) {
		zval *a1 = ZEND_CALL_ARG(execute_data, 1);
		if (a1 && Z_TYPE_P(a1) == IS_OBJECT) {
			event = a1;
		}
	}
	if (!event) {
		return;
	}
	zval *name = ZEND_CALL_NUM_ARGS(execute_data) >= 2 ? ZEND_CALL_ARG(execute_data, 2) : NULL;
	lerd_ensure_collector();
	zval fname, rv, args[2];
	ZVAL_STRINGL(&fname, "Lerd\\Collector\\event", sizeof("Lerd\\Collector\\event") - 1);
	ZVAL_COPY(&args[0], event);
	if (name && Z_TYPE_P(name) == IS_STRING) {
		ZVAL_COPY(&args[1], name);
	} else {
		ZVAL_NULL(&args[1]);
	}
	if (call_user_function(NULL, NULL, &fname, &rv, 2, args) == SUCCESS) {
		zval_ptr_dtor(&rv);
	}
	zval_ptr_dtor(&fname);
	zval_ptr_dtor(&args[0]);
	zval_ptr_dtor(&args[1]);
}

/* lerd_job_end captures one message dispatched to the Symfony Messenger bus.
 * dispatch() returns a new Envelope (not its argument), so arg1 — the message —
 * survives and we read it directly. Laravel claims jobs via its adapter. */
static void lerd_job_end(zend_execute_data *execute_data, zval *retval)
{
	(void)retval;
	if (!LERD_G(active) || lerd_laravel || (lerd_is_worker && !LERD_G(capture_workers))) {
		return;
	}
	if (ZEND_CALL_NUM_ARGS(execute_data) < 1) {
		return;
	}
	zval *msg = ZEND_CALL_ARG(execute_data, 1);
	if (!msg || Z_TYPE_P(msg) != IS_OBJECT) {
		return;
	}
	lerd_ensure_collector();
	zval fname, rv, args[1];
	ZVAL_STRINGL(&fname, "Lerd\\Collector\\job", sizeof("Lerd\\Collector\\job") - 1);
	ZVAL_COPY(&args[0], msg);
	if (call_user_function(NULL, NULL, &fname, &rv, 1, args) == SUCCESS) {
		zval_ptr_dtor(&rv);
	}
	zval_ptr_dtor(&fname);
	zval_ptr_dtor(&args[0]);
}

/* lerd_http_begin captures one outgoing Symfony HttpClient request. We hook the
 * begin (not the end) because request() rewrites its own $url argument via
 * prepareRequest(); at begin the original method and url are still intact. */
static void lerd_http_begin(zend_execute_data *execute_data)
{
	if (!LERD_G(active) || lerd_laravel || (lerd_is_worker && !LERD_G(capture_workers))) {
		return;
	}
	if (ZEND_CALL_NUM_ARGS(execute_data) < 2) {
		return;
	}
	zval *method = ZEND_CALL_ARG(execute_data, 1);
	zval *url = ZEND_CALL_ARG(execute_data, 2);
	if (!url || Z_TYPE_P(url) != IS_STRING) {
		return;
	}
	lerd_ensure_collector();
	zval fname, rv, args[2];
	ZVAL_STRINGL(&fname, "Lerd\\Collector\\http", sizeof("Lerd\\Collector\\http") - 1);
	ZVAL_COPY(&args[0], method);
	ZVAL_COPY(&args[1], url);
	if (call_user_function(NULL, NULL, &fname, &rv, 2, args) == SUCCESS) {
		zval_ptr_dtor(&rv);
	}
	zval_ptr_dtor(&fname);
	zval_ptr_dtor(&args[0]);
	zval_ptr_dtor(&args[1]);
}

static zend_observer_fcall_handlers lerd_observer_init(zend_execute_data *execute_data)
{
	zend_observer_fcall_handlers h = {NULL, NULL};
	zend_function *fn = execute_data ? execute_data->func : NULL;
	if (!fn || !fn->common.function_name || !fn->common.scope) {
		return h;
	}
	const char *fname = ZSTR_VAL(fn->common.function_name);
	zend_class_entry *scope = fn->common.scope;
	if (strcasecmp(ZSTR_VAL(scope->name), "Illuminate\\Foundation\\Application") == 0 &&
		strcasecmp(fname, "boot") == 0) {
		h.end = lerd_boot_end;
		return h;
	}
	if (zend_string_equals_literal_ci(scope->name, "PDOStatement") &&
		(strcasecmp(fname, "bindvalue") == 0 || strcasecmp(fname, "bindparam") == 0)) {
		h.end = lerd_bind_end;
		return h;
	}
	/* Agnostic mail: Symfony Mailer is the de-facto mail library, used directly
	 * by Symfony and wrapped by Laravel, so one seam covers both. */
	if (strcasecmp(ZSTR_VAL(scope->name), "Symfony\\Component\\Mailer\\Mailer") == 0 &&
		strcasecmp(fname, "send") == 0) {
		h.end = lerd_mail_end;
		return h;
	}
	/* Agnostic views: Twig is the de-facto Symfony view layer. render() returns
	 * the markup, display() echoes it; neither nests in the other, so observing
	 * both public entry points captures each render exactly once. */
	if (strcasecmp(ZSTR_VAL(scope->name), "Twig\\Environment") == 0 &&
		(strcasecmp(fname, "render") == 0 || strcasecmp(fname, "display") == 0)) {
		h.end = lerd_view_end;
		return h;
	}
	/* Agnostic events: the Symfony EventDispatcher is the de-facto event bus.
	 * The debug TraceableEventDispatcher delegates to this concrete class, so
	 * observing it captures each dispatch once. */
	if (strcasecmp(ZSTR_VAL(scope->name), "Symfony\\Component\\EventDispatcher\\EventDispatcher") == 0 &&
		strcasecmp(fname, "dispatch") == 0) {
		h.end = lerd_event_end;
		return h;
	}
	/* Agnostic jobs: the Symfony Messenger bus is the de-facto queue entry. The
	 * debug TraceableMessageBus delegates to this concrete class, so each
	 * dispatch is seen once. */
	if (strcasecmp(ZSTR_VAL(scope->name), "Symfony\\Component\\Messenger\\MessageBus") == 0 &&
		strcasecmp(fname, "dispatch") == 0) {
		h.end = lerd_job_end;
		return h;
	}
	/* Agnostic outgoing HTTP: the default Symfony HttpClient factory yields one
	 * of these two concrete clients; decorators (traceable, retryable, scoping)
	 * delegate down to them, so the real request is seen once. */
	if ((strcasecmp(ZSTR_VAL(scope->name), "Symfony\\Component\\HttpClient\\CurlHttpClient") == 0 ||
		 strcasecmp(ZSTR_VAL(scope->name), "Symfony\\Component\\HttpClient\\NativeHttpClient") == 0) &&
		strcasecmp(fname, "request") == 0) {
		h.begin = lerd_http_begin;
		return h;
	}
	zend_bool match =
		(zend_string_equals_literal_ci(scope->name, "PDOStatement") && strcasecmp(fname, "execute") == 0) ||
		(zend_string_equals_literal_ci(scope->name, "PDO") &&
			(strcasecmp(fname, "query") == 0 || strcasecmp(fname, "exec") == 0));
	if (match) {
		h.begin = lerd_obs_begin;
		h.end = lerd_obs_end;
	}
	return h;
}
#endif /* LERD_OBSERVE */

/* ── module lifecycle ───────────────────────────────────────────────────── */

static PHP_GINIT_FUNCTION(lerd_devtools)
{
#if defined(COMPILE_DL_LERD_DEVTOOLS) && defined(ZTS)
	ZEND_TSRMLS_CACHE_UPDATE();
#endif
	memset(lerd_devtools_globals, 0, sizeof(*lerd_devtools_globals));
}

PHP_MINIT_FUNCTION(lerd_devtools)
{
	REGISTER_INI_ENTRIES();
#ifdef LERD_OBSERVE
	detect_worker();
	zend_observer_fcall_register(lerd_observer_init);
#endif
	return SUCCESS;
}

PHP_MSHUTDOWN_FUNCTION(lerd_devtools)
{
	UNREGISTER_INI_ENTRIES();
	return SUCCESS;
}

PHP_RINIT_FUNCTION(lerd_devtools)
{
#if defined(ZTS) && defined(COMPILE_DL_LERD_DEVTOOLS)
	ZEND_TSRMLS_CACHE_UPDATE();
#endif
#ifdef LERD_OBSERVE
	LERD_G(tstack_len) = 0;
	LERD_G(collector_loaded) = 0;
	LERD_G(bind_buf_init) = 0;
	LERD_G(is_test) = 0;
	/* Reset per request: FPM reuses the worker process across requests with a
	 * fresh app each time, so the adapter must be re-loaded (and the PDO
	 * suppression re-armed) on every request, not just the first. */
	lerd_laravel = 0;
	LERD_G(active) = (LERD_G(flag) && LERD_G(flag)[0] && access(LERD_G(flag), F_OK) == 0) ? 1 : 0;
	LERD_G(want_query) = (LERD_G(kinds) && strstr(LERD_G(kinds), "query")) ? 1 : 0;
	LERD_G(capture_workers) = (access("/usr/local/etc/lerd/devtools-workers.flag", F_OK) == 0) ? 1 : 0;
	/* One id per RINIT: per HTTP request under FPM, per process under CLI. The
	 * dashboard groups queries by this so every request is its own group, even
	 * two hits to the same URL on the same reused pool worker. time+pid+seq is
	 * unique without needing an RNG. */
	{
		struct timespec rts;
		clock_gettime(CLOCK_REALTIME, &rts);
		unsigned long long us = (unsigned long long)rts.tv_sec * 1000000ULL + (unsigned long long)(rts.tv_nsec / 1000);
		snprintf(LERD_G(rid), sizeof(LERD_G(rid)), "%015llx%lx%lx", us, (unsigned long)getpid(), (lerd_seq++ & 0xffffUL));
	}
	/* Expose the per-request capture decision + worker name to the Laravel
	 * adapter (PHP), so it applies the same on/off and worker policy as the
	 * engine-level path without re-deriving it. Request-scoped constants. */
	{
		zend_bool on = LERD_G(active) && LERD_G(want_query) && (!lerd_is_worker || LERD_G(capture_workers));
		zend_register_bool_constant("LERD_DEVTOOLS_ON", sizeof("LERD_DEVTOOLS_ON") - 1, on, 0, module_number);
		zend_register_string_constant("LERD_DEVTOOLS_WORKER", sizeof("LERD_DEVTOOLS_WORKER") - 1, lerd_worker_cmd, 0, module_number);
		/* The agnostic collector groups its events with this request's queries. */
		zend_register_string_constant("LERD_DEVTOOLS_RID", sizeof("LERD_DEVTOOLS_RID") - 1, LERD_G(rid), 0, module_number);
	}
#endif
	return SUCCESS;
}

PHP_RSHUTDOWN_FUNCTION(lerd_devtools)
{
#ifdef LERD_OBSERVE
	if (LERD_G(bind_buf_init)) {
		zend_hash_destroy(&LERD_G(bind_buf));
		LERD_G(bind_buf_init) = 0;
	}
#endif
	return SUCCESS;
}

PHP_GSHUTDOWN_FUNCTION(lerd_devtools)
{
#ifdef LERD_OBSERVE
	if (lerd_devtools_globals->tstack) {
		free(lerd_devtools_globals->tstack);
		lerd_devtools_globals->tstack = NULL;
	}
#endif
}

PHP_MINFO_FUNCTION(lerd_devtools)
{
	php_info_print_table_start();
	php_info_print_table_header(2, "lerd_devtools support", "enabled");
	php_info_print_table_row(2, "version", PHP_LERD_DEVTOOLS_VERSION);
#ifdef LERD_OBSERVE
	php_info_print_table_row(2, "capture", "zend_observer (PHP 8.0+)");
#else
	php_info_print_table_row(2, "capture", "disabled (needs PHP 8.0+)");
#endif
	php_info_print_table_end();
	DISPLAY_INI_ENTRIES();
}

zend_module_entry lerd_devtools_module_entry = {
	STANDARD_MODULE_HEADER,
	"lerd_devtools",
	NULL, /* functions */
	PHP_MINIT(lerd_devtools),
	PHP_MSHUTDOWN(lerd_devtools),
	PHP_RINIT(lerd_devtools),
	PHP_RSHUTDOWN(lerd_devtools),
	PHP_MINFO(lerd_devtools),
	PHP_LERD_DEVTOOLS_VERSION,
	PHP_MODULE_GLOBALS(lerd_devtools),
	PHP_GINIT(lerd_devtools),
	PHP_GSHUTDOWN(lerd_devtools),
	NULL,
	STANDARD_MODULE_PROPERTIES_EX
};

#ifdef COMPILE_DL_LERD_DEVTOOLS
#ifdef ZTS
ZEND_TSRMLS_CACHE_DEFINE()
#endif
ZEND_GET_MODULE(lerd_devtools)
#endif
