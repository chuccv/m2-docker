#!/bin/bash
set -euo pipefail
v="${1:?php version}"
dir="$(cd "$(dirname "$0")/.." && pwd)"
tag="md/php:$v"

build() {
    docker build --build-arg PHP_VERSION="$v" -t "$tag" "$dir"
    docker image prune -f --filter label=com.md.image=php >/dev/null
}

fail() { echo "FAIL: $*" >&2; exit 1; }

restore() { sed -i '${/^$/d}' "$dir/nginx.conf"; }
trap restore EXIT

build
echo >> "$dir/nginx.conf"
build
restore

run() { docker run --rm "$tag" "$@"; }

mods="$(run php -m | tr 'A-Z' 'a-z')"
for e in bcmath ctype curl dom gd intl mbstring pdo_mysql simplexml soap sockets sodium xsl zip "zend opcache" redis; do
    grep -qx "$e" <<<"$mods" || fail "missing extension $e"
done
for e in xdebug xhprof; do
    grep -qx "$e" <<<"$mods" && fail "$e should be disabled by default"
done
run php -d extension=xhprof -m | grep -qix xhprof || fail "xhprof not loadable"
[ "$(run php -d extension=xhprof -d auto_prepend_file=/usr/local/share/php-profiler/prepend.php -r 'var_export(xhprof_disable());')" = "NULL" ] || fail "cli is profiled"
run php -d zend_extension=xdebug -m | grep -qix xdebug || fail "xdebug not loadable"
[ "$(run php -r 'echo ini_get("memory_limit");')" = "-1" ] || fail memory_limit
[ "$(run id -u)" = "1000" ] || fail uid
[ "$(run id -g)" = "1000" ] || fail gid
for c in mailpit composer mysql nginx apachectl php; do
    run sh -c "command -v $c" >/dev/null || fail "missing $c"
done
run sh -c 'ls /usr/local/etc/php/conf.d/xdebug.ini.off /usr/local/etc/php/conf.d/xhprof.ini.off /usr/local/share/php-profiler/prepend.php /usr/local/share/php-profiler/vendor/autoload.php' >/dev/null || fail "off ini or profiler missing"
run sh -c 'command -v pkill && touch /usr/local/etc/php/conf.d/.w && rm /usr/local/etc/php/conf.d/.w' >/dev/null || fail "conf.d not writable or pkill missing"
run grep -q 'max_children = 16' /usr/local/etc/php-fpm.d/www.conf || fail max_children
[ "$(run pwd)" = "/var/www/html" ] || fail workdir

tmp="$(mktemp -d)"
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$tmp/k.pem" -out "$tmp/md.crt" -subj "/CN=md-test-ca" -days 1 >/dev/null 2>&1
mkdir -p "$tmp/www/pub"
printf '<?php echo $_SERVER["MAGE_RUN_CODE"] ?? "unset", "|", $_SERVER["MAGE_RUN_TYPE"] ?? "unset";' > "$tmp/www/pub/index.php"
printf 'map $host $md_run_code {\n    default "";\n    b2b.shop.ddev.site "b2b";\n}\nmap $host $md_run_type {\n    default "";\n    b2b.shop.ddev.site "website";\n}\n' > "$tmp/md-run.conf"
printf '<?php echo $_SERVER["HTTP_X_FORWARDED_PROTO"] ?? "unset";' > "$tmp/www/pub/get.php"
printf '<?php echo $_SERVER["SERVER_NAME"];' > "$tmp/www/pub/health_check.php"
cid="$(docker run -d -v "$tmp/www:/var/www/html" -v "$tmp/md-run.conf:/etc/nginx/md-run.conf:ro" -v "$tmp/md.crt:/usr/local/share/ca-certificates/md.crt:ro" "$tag")"
trap 'docker rm -f "$cid" >/dev/null 2>&1; rm -rf "$tmp"; restore' EXIT
sleep 5
[ "$(docker exec "$cid" curl -s -H 'X-Forwarded-Proto: https' http://127.0.0.1/get.php)" = "https" ] || fail "nginx X-Forwarded-Proto not passed"
docker exec "$cid" touch /var/www/html/x
[ "$(docker exec "$cid" stat -c %u /var/www/html/x)" = "1000" ] || fail "exec file not uid 1000"
[ "$(docker exec "$cid" id -u)" = "1000" ] || fail "exec uid"
docker exec "$cid" sh -c 'grep -c . /etc/ssl/certs/ca-certificates.crt >/dev/null; ls /etc/ssl/certs/md.pem' >/dev/null || fail "CA not installed"
docker exec "$cid" sh -c 'openssl verify -CAfile /etc/ssl/certs/ca-certificates.crt /usr/local/share/ca-certificates/md.crt' >/dev/null 2>&1 || fail "CA not in bundle"
docker exec "$cid" sh -c 'curl -fs -o /dev/null -w "%{http_code}" http://127.0.0.1:8025/ | grep -q 200' || fail "mailpit UI"
docker exec "$cid" sh -c 'curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1/ | grep -Eq "^(200|404|403|50[0-9])$"' || fail "nginx not responding"
[ "$(docker exec "$cid" curl -s -H 'Host: b2b.shop.ddev.site' http://127.0.0.1/index.php)" = "b2b|website" ] || fail "MAGE_RUN_CODE not set for b2b host"
[ "$(docker exec "$cid" curl -s -H 'Host: shop.ddev.site' http://127.0.0.1/index.php)" = "|" ] || fail "MAGE_RUN_CODE not empty for default host"
[ "$(docker exec "$cid" curl -s -H 'Host: shop.md.site' http://127.0.0.1/health_check.php)" = "shop.md.site" ] || fail "nginx SERVER_NAME not host"
procs="$(docker exec "$cid" sh -c 'for d in /proc/[0-9]*; do echo "$(cat $d/comm) $(stat -c %U $d)"; done')"
for p in php-fpm nginx mailpit; do
    grep -qx "$p www" <<<"$procs" || fail "$p not running as www"
done

docker exec "$cid" sh -c 'kill $(for d in /proc/[0-9]*; do [ "$(cat $d/comm)" = php-fpm ] && echo ${d#/proc/}; done)'
sleep 3
[ "$(docker inspect -f '{{.State.Running}}' "$cid")" = "false" ] || fail "container survived php-fpm kill"
[ "$(docker inspect -f '{{.State.ExitCode}}' "$cid")" != "0" ] || fail "exit code 0"

acid=""
trap 'docker rm -f "$cid" $acid >/dev/null 2>&1; rm -rf "$tmp"; restore' EXIT
mkdir -p "$tmp/awww/pub"
printf '<?php echo $_SERVER["MAGE_RUN_CODE"] ?? "unset", "|", $_SERVER["MAGE_RUN_TYPE"] ?? "unset";' > "$tmp/awww/pub/index.php"
printf '<?php echo "rewritten";' > "$tmp/awww/pub/real.php"
cp "$tmp/www/pub/get.php" "$tmp/www/pub/health_check.php" "$tmp/awww/pub/"
mkdir -p "$tmp/awww/pub/v24" "$tmp/awww/pub/v22"
printf '<IfVersion >= 2.4>\n    Require all denied\n</IfVersion>\n' > "$tmp/awww/pub/v24/.htaccess"
printf 'Order allow,deny\nDeny from all\n' > "$tmp/awww/pub/v22/.htaccess"
printf 'x' > "$tmp/awww/pub/v24/f.txt"
printf 'x' > "$tmp/awww/pub/v22/f.txt"
printf 'RewriteEngine On\nRewriteRule ^pretty$ real.php [L]\n' > "$tmp/awww/pub/.htaccess"
printf 'SetEnvIfNoCase Host "^b2b[.]shop[.]ddev[.]site(:[0-9]+)?$" MAGE_RUN_CODE=b2b MAGE_RUN_TYPE=website\n' > "$tmp/apache-run.conf"
acid="$(docker run -d -e MD_WEBSERVER=apache -v "$tmp/awww:/var/www/html" -v "$tmp/apache-run.conf:/etc/apache2/md-run.conf:ro" "$tag")"
sleep 5
docker exec "$acid" apachectl -t >/dev/null 2>&1 || fail "apachectl -t"
[ "$(docker exec "$acid" curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1/index.php)" = "200" ] || fail "apache php not 200"
[ "$(docker exec "$acid" curl -s http://127.0.0.1/pretty)" = "rewritten" ] || fail "apache .htaccess rewrite"
[ "$(docker exec "$acid" curl -s -H 'Host: b2b.shop.ddev.site:8443' http://127.0.0.1/index.php)" = "b2b|website" ] || fail "apache MAGE_RUN_CODE for b2b host"
[ "$(docker exec "$acid" curl -s -H 'Host: shop.ddev.site' http://127.0.0.1/index.php)" = "unset|unset" ] || fail "apache MAGE_RUN_CODE for default host"
[ "$(docker exec "$acid" curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1/v24/f.txt)" = "403" ] || fail "apache IfVersion/Require htaccess not 403"
[ "$(docker exec "$acid" curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1/v22/f.txt)" = "403" ] || fail "apache Order/Deny htaccess not 403"
[ "$(docker exec "$acid" curl -s -H 'X-Forwarded-Proto: https' http://127.0.0.1/get.php)" = "https" ] || fail "apache X-Forwarded-Proto not passed"
[ "$(docker exec "$acid" curl -s -H 'Host: shop.md.site' http://127.0.0.1/health_check.php)" = "shop.md.site" ] || fail "apache SERVER_NAME not host"
aprocs="$(docker exec "$acid" sh -c 'for d in /proc/[0-9]*; do echo "$(cat $d/comm) $(stat -c %U $d)"; done')"
grep -qx "apache2 www" <<<"$aprocs" || fail "apache2 not running as www"
grep -q "^nginx " <<<"$aprocs" && fail "nginx running in apache mode"
printf 'SetEnvIfNoCase Host "^c2c[.]shop[.]ddev[.]site$" MAGE_RUN_CODE=c2c MAGE_RUN_TYPE=website\n' > "$tmp/apache-run.conf"
docker exec "$acid" apachectl -k graceful >/dev/null 2>&1 || fail "apachectl graceful"
sleep 2
[ "$(docker exec "$acid" curl -s -H 'Host: c2c.shop.ddev.site' http://127.0.0.1/index.php)" = "c2c|website" ] || fail "apache graceful did not reload map"
docker rm -f "$acid" >/dev/null
acid=""

lines="$(docker images --filter label=com.md.image=php --format '{{.Repository}}:{{.Tag}}' | grep -c "^md/php:$v\$" || true)"
[ "$lines" = "1" ] || fail "expected 1 image for $v, got $lines"
[ -z "$(docker images --filter label=com.md.image=php --filter dangling=true -q)" ] || fail "dangling php images"
docker images "$tag" --format '{{.Repository}}:{{.Tag}} {{.Size}}'
echo "PASS $v"
