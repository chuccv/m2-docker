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
run php -d zend_extension=xdebug -m | grep -qix xdebug || fail "xdebug not loadable"
[ "$(run php -r 'echo ini_get("memory_limit");')" = "-1" ] || fail memory_limit
[ "$(run id -u)" = "1000" ] || fail uid
[ "$(run id -g)" = "1000" ] || fail gid
for c in mailpit composer mysql nginx php; do
    run sh -c "command -v $c" >/dev/null || fail "missing $c"
done
run sh -c 'ls /usr/local/etc/php/conf.d/xdebug.ini.off /usr/local/etc/php/conf.d/xhprof.ini.off /usr/local/share/php-profiler/prepend.php /usr/local/share/php-profiler/vendor/autoload.php' >/dev/null || fail "off ini or profiler missing"
run sh -c 'command -v pkill && touch /usr/local/etc/php/conf.d/.w && rm /usr/local/etc/php/conf.d/.w' >/dev/null || fail "conf.d not writable or pkill missing"
run grep -q 'max_children = 16' /usr/local/etc/php-fpm.d/www.conf || fail max_children
[ "$(run pwd)" = "/var/www/html" ] || fail workdir

tmp="$(mktemp -d)"
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$tmp/k.pem" -out "$tmp/md.crt" -subj "/CN=md-test-ca" -days 1 >/dev/null 2>&1
mkdir "$tmp/www"
cid="$(docker run -d -v "$tmp/www:/var/www/html" -v "$tmp/md.crt:/usr/local/share/ca-certificates/md.crt:ro" "$tag")"
trap 'docker rm -f "$cid" >/dev/null 2>&1; rm -rf "$tmp"; restore' EXIT
sleep 5
docker exec "$cid" touch /var/www/html/x
[ "$(docker exec "$cid" stat -c %u /var/www/html/x)" = "1000" ] || fail "exec file not uid 1000"
[ "$(docker exec "$cid" id -u)" = "1000" ] || fail "exec uid"
docker exec "$cid" sh -c 'grep -c . /etc/ssl/certs/ca-certificates.crt >/dev/null; ls /etc/ssl/certs/md.pem' >/dev/null || fail "CA not installed"
docker exec "$cid" sh -c 'openssl verify -CAfile /etc/ssl/certs/ca-certificates.crt /usr/local/share/ca-certificates/md.crt' >/dev/null 2>&1 || fail "CA not in bundle"
docker exec "$cid" sh -c 'curl -fs -o /dev/null -w "%{http_code}" http://127.0.0.1:8025/ | grep -q 200' || fail "mailpit UI"
docker exec "$cid" sh -c 'curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1/ | grep -Eq "^(404|403|50[0-9])$"' || fail "nginx not responding"
procs="$(docker exec "$cid" sh -c 'for d in /proc/[0-9]*; do echo "$(cat $d/comm) $(stat -c %U $d)"; done')"
for p in php-fpm nginx mailpit; do
    grep -qx "$p www" <<<"$procs" || fail "$p not running as www"
done

docker exec "$cid" sh -c 'kill $(for d in /proc/[0-9]*; do [ "$(cat $d/comm)" = php-fpm ] && echo ${d#/proc/}; done)'
sleep 3
[ "$(docker inspect -f '{{.State.Running}}' "$cid")" = "false" ] || fail "container survived php-fpm kill"
[ "$(docker inspect -f '{{.State.ExitCode}}' "$cid")" != "0" ] || fail "exit code 0"

lines="$(docker images --filter label=com.md.image=php --format '{{.Repository}}:{{.Tag}}' | grep -c "^md/php:$v\$" || true)"
[ "$lines" = "1" ] || fail "expected 1 image for $v, got $lines"
[ -z "$(docker images --filter label=com.md.image=php --filter dangling=true -q)" ] || fail "dangling php images"
docker images "$tag" --format '{{.Repository}}:{{.Tag}} {{.Size}}'
echo "PASS $v"
