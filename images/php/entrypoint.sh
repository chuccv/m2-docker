#!/bin/bash
if [ "$#" -gt 0 ]; then
    exec "$@"
fi
update-ca-certificates >/dev/null 2>&1 || true
php-fpm &
pids="$!"
/usr/local/bin/mailpit --smtp 0.0.0.0:1025 --listen 0.0.0.0:8025 &
pids="$pids $!"
if [ "${MD_WEBSERVER:-nginx}" = "apache" ]; then
    apachectl -D FOREGROUND &
else
    nginx -g 'daemon off;' &
fi
pids="$pids $!"
if [ "${MD_CRON:-0}" = "1" ]; then
    supercronic -quiet /etc/md/crontab &
fi
wait -n $pids
exit 1
