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

## V1 limits

- MariaDB only (no MySQL)
- nginx only
- No varnish, rabbitmq, elasticsearch or mutagen
- Only the DDEV add-ons opensearch, redis and xhgui are understood

## Compatibility with DDEV

- Container names stay `ddev-<name>-web`, `ddev-<name>-db`, `ddev-<name>-redis`
- Hostnames stay `<name>.ddev.site`, plus `additional_hostnames` and `additional_fqdns`
- Existing `<name>-mariadb` volumes are reused as-is
- md refuses to start while DDEV containers for the same site, or the DDEV router on the same ports, are running
