/* lerd_devtools — engine-level capture for the lerd debug window.
 *
 * Observes database calls (PDO today; mysqli/curl/exceptions in later
 * phases) via the zend_observer API and ships each as a newline-delimited
 * JSON event to the same Unix/TCP socket the debug bridge uses, where lerd-ui
 * buffers and fans it out. Capture is gated per request by a sentinel flag
 * file so toggling never restarts FPM, mirroring the debug bridge. Builds as a
 * no-op on PHP < 8.0 (no zend_observer) so the multi-version image build never
 * fails on the legacy tier.
 */
#ifndef PHP_LERD_DEVTOOLS_H
#define PHP_LERD_DEVTOOLS_H

extern zend_module_entry lerd_devtools_module_entry;
#define phpext_lerd_devtools_ptr &lerd_devtools_module_entry

#define PHP_LERD_DEVTOOLS_VERSION "0.1.0"

#include "php.h"

ZEND_BEGIN_MODULE_GLOBALS(lerd_devtools)
	char *host;       /* lerd.devtools_host: unix:///… or tcp://host:port */
	char *kinds;      /* lerd.devtools_kinds: comma list, e.g. "query"    */
	char *flag;       /* lerd.devtools_flag: sentinel path, stat per req  */
	zend_bool active; /* flag present this request (cached at RINIT)      */
	zend_bool want_query;
	zend_bool capture_workers; /* devtools-workers.flag present (RINIT)    */
	char rid[48];     /* unique per request/invocation, stamped at RINIT  */
	double *tstack;   /* call-start timestamps, indexed by observed depth */
	int tstack_len;
	int tstack_cap;
	HashTable bind_buf;       /* PDOStatement obj-handle -> array of bound values */
	zend_bool bind_buf_init;  /* whether bind_buf is initialised this request    */
	zend_bool collector_loaded; /* agnostic PHP collector included this request   */
	zend_bool is_test;          /* PHPUnit/Pest run detected (sticky per request) */
ZEND_END_MODULE_GLOBALS(lerd_devtools)

#if defined(ZTS) && defined(COMPILE_DL_LERD_DEVTOOLS)
ZEND_TSRMLS_CACHE_EXTERN()
#endif

#define LERD_G(v) ZEND_MODULE_GLOBALS_ACCESSOR(lerd_devtools, v)

#endif /* PHP_LERD_DEVTOOLS_H */
