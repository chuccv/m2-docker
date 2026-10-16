#!/bin/bash
if [ "$#" -gt 0 ]; then
    exec "$@"
fi
update-ca-certificates >/dev/null 2>&1 || true
php-fpm &
/usr/local/bin/mailpit --smtp 0.0.0.0:1025 --listen 0.0.0.0:8025 &
if [ "${MD_WEBSERVER:-nginx}" = "apache" ]; then
    apachectl -D FOREGROUND &
else
    nginx -g 'daemon off;' &
fi
wait -n
exit 1
