ARG PHP_VERSION=8.5
FROM php:${PHP_VERSION}-fpm-bookworm

LABEL com.md.image=php

ARG IPE_VERSION=2.12.1
ARG MAILPIT_VERSION=v1.31.6

ADD https://github.com/mlocati/docker-php-extension-installer/releases/download/${IPE_VERSION}/install-php-extensions /usr/local/bin/install-php-extensions

RUN chmod +x /usr/local/bin/install-php-extensions \
 && apt-get update \
 && apt-get install -y --no-install-recommends nginx apache2 tini mariadb-client git unzip ca-certificates curl util-linux procps \
 && install-php-extensions @composer bcmath gd intl pdo_mysql soap sockets sodium xsl zip opcache redis xdebug xhprof \
 && rm -f /usr/local/etc/php/conf.d/docker-php-ext-xdebug.ini /usr/local/etc/php/conf.d/docker-php-ext-xhprof.ini \
 && arch="$(dpkg --print-architecture)" \
 && curl -fsSL "https://github.com/axllent/mailpit/releases/download/${MAILPIT_VERSION}/mailpit-linux-${arch}.tar.gz" | tar -xz -C /usr/local/bin mailpit \
 && groupadd -g 1000 www \
 && useradd -u 1000 -g 1000 -m -s /bin/bash www \
 && mkdir -p /usr/local/share/php-profiler /var/www/html \
 && COMPOSER_ALLOW_SUPERUSER=1 composer require perftools/php-profiler --working-dir=/usr/local/share/php-profiler --no-interaction --no-progress \
 && composer clear-cache \
 && chown -R www:www /var/www /etc/ssl/certs /usr/local/share/ca-certificates \
 && chown www:www /etc/ssl/certs/ca-certificates.crt \
 && rm -rf /var/lib/apt/lists/* /tmp/* /usr/share/doc /usr/share/man /var/cache/apt

COPY nginx.conf /etc/nginx/nginx.conf
COPY md-run.conf /etc/nginx/md-run.conf
COPY apache2.conf /etc/apache2/apache2.conf
COPY pool.conf /usr/local/etc/php-fpm.d/www.conf
COPY conf.d/ /usr/local/etc/php/conf.d/
COPY prepend.php /usr/local/share/php-profiler/prepend.php
COPY entrypoint.sh /usr/local/bin/md-entrypoint
RUN : > /etc/apache2/md-run.conf \
 && printf "export APACHE_RUN_USER=www\\nexport APACHE_RUN_GROUP=www\\nexport APACHE_RUN_DIR=/tmp/apache2\\nexport APACHE_PID_FILE=/tmp/apache2.pid\\nexport APACHE_LOCK_DIR=/tmp\\nexport APACHE_LOG_DIR=/tmp\\nexport LANG=C\\n" > /etc/apache2/envvars \
 && rm -f /usr/local/etc/php-fpm.d/zz-docker.conf /usr/local/etc/php-fpm.d/docker.conf \
 && chmod +x /usr/local/bin/md-entrypoint \
 && chown www:www /usr/local/etc/php/conf.d

ENV PATH=/opt/node/bin:$PATH
WORKDIR /var/www/html
EXPOSE 80 8025 1025
USER www
ENTRYPOINT ["tini", "-g", "--", "/usr/local/bin/md-entrypoint"]
