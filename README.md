# md

Local Magento 2 sites on plain Docker Compose + Traefik. Replaces DDEV, keeps its container names, hostnames and DB volumes.

Needs Linux, Docker Compose v2, mkcert.

    go install github.com/chuccv/m2-docker/cmd/md@latest
    md trust                 # local CA, once

## Setting up a site

| Folder | Command |
|---|---|
| Empty | `md install [dir] --version 2.4.8` |
| Has `.ddev` | `md migrate [dir]` |
| Has Magento code | `md init [dir]` |

`md init` reads the version from `composer.lock`, writes `.md/config.yaml`, runs `up`, checks the domain answers. Then import the DB:

```bash
gunzip -c dump.sql.gz | md mysql
md magento setup:config:set ...      # init prints the full command
md magento setup:upgrade
```

## Daily use

    md up | stop | restart   # this site
    md poweroff              # everything
    md magento <args>
    md exec <cmd>
    md mysql
    md snapshot [--list | restore <file>]
    md xhprof on|off         # XHGui: https://<host>:8142
    md xdebug on|off
    md ui                    # dashboard, 127.0.0.1:9900
    md clean

Outside the site folder: `md -s <name> <cmd>`.

## Domains

Sites answer on `<name>.<domain>`, `<name>.md.site`, `<name>.ddev.site`.

- `ddev.site`: public DNS, nothing to do.
- `md.site`: needs `/etc/hosts` (`md hosts --all | sudo tee -a /etc/hosts`).
- Own domain: set `domain:` in `~/.md/config.yaml` and add DNS `A * -> 127.0.0.1` (not proxied). No `/etc/hosts`.

## Config

`~/.md/config.yaml`, all optional:

    domain: md.site
    bind_all_interfaces: false
    ports: {http: 80, https: 443, mailpit_https: 8026, xhgui_https: 8142, phpmyadmin_https: 8037}

Stacks per Magento version: `internal/install/stacks.yaml`, override in `~/.md/stacks.yaml`.

## MCP

    claude mcp add -s user m2-docker -- md mcp

Tools: `sites_list`, `site_describe`, `site_init`, `site_up`, `site_stop`, `site_restart`, `magento`, `exec`, `sql`, `logs`, `snapshot_*`, `xdebug`, `xhprof`.

Design notes: `docs/design.md`.
