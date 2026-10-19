# md

`md` is a small Go CLI that runs local Magento 2 sites from an existing DDEV project config, replacing the DDEV router and containers with plain Docker Compose and Traefik.

## Requirements

- Linux
- Docker with Compose v2
- mkcert

## Install

    go install github.com/chuccv/m2-docker/cmd/md@latest

## Quickstart

    md trust                 # install the local CA once
    cd /path/to/site         # a directory with .ddev/config.yaml
    md up                    # start the site, print its URLs
    md exec <cmd>            # run a command in the web container
    md magento <args>        # bin/magento
    md mysql                 # mariadb client in the db container
    md stop                  # stop this site
    md poweroff              # stop everything, including the router
    md snapshot              # dump the database (--list, restore <file>)
    md clean                 # remove unused images, build cache, volumes

Commands find the site by walking up from the current directory to `.md/config.yaml` or `.ddev/config.yaml`, then use its `name` for container names (`ddev-<name>-db` for `md mysql`). From anywhere else pass `-s <name>`, e.g. `md -s shop mysql`.

State lives in `~/.md`. Snapshots go to `~/.md/sites/<name>/snapshots/`.

## Configuration

`~/.md/config.yaml`, all keys optional:

    ports:
      http: 80
      https: 443
      mailpit_http: 8025
      mailpit_https: 8026
      xhgui_http: 8143
      xhgui_https: 8142
    bind_all_interfaces: false   # true publishes router ports on 0.0.0.0, default is 127.0.0.1

## MCP server

`md mcp` serves an MCP server over stdio so Claude Code can drive md and Magento sites.

    claude mcp add -s user m2 -- md mcp

Every tool except `sites_list` takes the site name as `site`. Output is capped at about 20 KB.

- `sites_list`, `site_describe`
- `site_up`, `site_stop`, `site_restart`
- `magento` (`args`), `exec` (`cmd`, run with `bash -lc`), `sql` (`query`, writes allowed)
- `logs` (`file`: `exception`, `system`, `debug` or `report` with `id`; `lines`, `grep`)
- `snapshot_create`, `snapshot_list`, `snapshot_restore`
- `xdebug`, `xhprof` (`on`; xhprof returns the XHGui URL)

There are no tools for `clean`, `poweroff`, `install` or `migrate`.

## Supported

- Web server: nginx-fpm or apache-fpm, PHP 8.1 to 8.5
- Database: MariaDB or MySQL
- Search: OpenSearch or Elasticsearch
- Redis, Varnish, phpMyAdmin, Node, Mailpit, Xdebug, XHProf
- Not supported: RabbitMQ, custom DDEV compose files (`md migrate --ignore-extra-compose` skips them)

## How it works

md is one static Go binary. It writes plain Docker Compose files and lets Docker do the rest; there is no daemon of its own.

| Piece | What md uses | Why |
|---|---|---|
| CLI | Go, cobra | single binary, no runtime |
| Orchestration | `docker compose` v2, one project per site (`md-<name>`) plus `md-shared` | stock tooling, easy to inspect with `docker compose ls` |
| Routing | one Traefik v3 container (`md-router`) with a file-provider dynamic config | one router for every site, hostnames go to the right web container |
| TLS | mkcert local CA, one certificate covering all site hostnames | green padlock in browsers and Playwright |
| Web image | `md/php:<version>` built locally from an embedded Dockerfile, shared by every site on that PHP version; nginx or apache, php-fpm and Mailpit run under tini | one image per PHP version instead of one per site |
| Image rebuilds | the image carries a hash label of its build context; `md up` rebuilds only when the hash changes and prunes the old image | upgrades apply on the next `md up` |
| Database | official `mariadb`/`mysql` images, one container per site, data in the site's `<name>-mariadb` volume | sites can pin different versions; DDEV volumes are reused without copying |
| Search | one shared OpenSearch/Elasticsearch container per version (`md-opensearch-<v>`, heap 512 MB, 768 MB for `latest`), each site gets its own index prefix, set once by `md install`, `md migrate` or the first `md up` of an installed site | one JVM instead of one per site |
| Node | the official `node:<major>-slim` files copied once into a `md-node-<major>` volume, mounted read-only into web containers | no Node install in the PHP image |
| Profiling | XHProf with perftools/php-profiler, one shared XHGui container | `md xhprof on` and open XHGui on the site hostname |
| Snapshots | `mariadb-dump`/`mysqldump` piped through gzip to `~/.md/sites/<name>/snapshots/` | restore takes a pre-restore snapshot first |
| Config | `.md/config.yaml` in the site, generated from `.ddev/config.yaml` by `md migrate` | `.ddev/` is never written |
| Magento settings | `bin/magento config:set --lock-env` for base URLs and search prefix | changes land in `env.php`, not the database |
| GUI | `md ui`: stdlib HTTP server with embedded HTML/JS, live stats over Server-Sent Events from one shared `docker stats` and `docker events` stream | loopback only, actions need a custom header |
| AI | `md mcp`: MCP server over stdio using the official Go SDK | lets an assistant run md and Magento commands |

Containers use `restart: "no"`: nothing starts on boot, run `md up` when you need a site.

## Compatibility with DDEV

- Container names stay `ddev-<name>-web`, `ddev-<name>-db`, `ddev-<name>-redis`
- Hostnames stay `<name>.ddev.site`, plus `additional_hostnames` and `additional_fqdns`
- Existing `<name>-mariadb` volumes are reused as-is
- md refuses to start while DDEV containers for the same site, or the DDEV router on the same ports, are running
