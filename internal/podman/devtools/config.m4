PHP_ARG_ENABLE([lerd_devtools],
  [whether to enable lerd_devtools support],
  [AS_HELP_STRING([--enable-lerd-devtools], [Enable lerd_devtools])],
  [yes])

if test "$PHP_LERD_DEVTOOLS" != "no"; then
  AC_DEFINE(HAVE_LERD_DEVTOOLS, 1, [ Have lerd_devtools support ])
  PHP_NEW_EXTENSION(lerd_devtools, lerd_devtools.c, $ext_shared)
fi
