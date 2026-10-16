# Task 7 report

RED: internal/cmd/exec_test.go written first; `go test ./internal/cmd` failed to compile (env, register undefined).
GREEN: after implementing exec.go, mysql.go, toggle.go, env in cmd.go; `go vet` and `go test ./...` all pass.

Design
- `env` struct (runner, tty detector, scanRoots, getwd, home, out, site) shared by all commands; `findSite` is the single site resolver (hosts.go now uses it).
- `-s/--site` persistent flag on root. Passthrough commands use DisableFlagParsing, so a leading `-s X`/`--site X`/`--site=X` after the subcommand is stripped manually (takeSite).
- tty: stdin and stdout both char devices.
- Exit code: main.go exits with *exec.ExitError.ExitCode() silently.
- Toggle: one `sh -c` script, no-op when the source file is absent, `mv` then `pkill -USR2 -o php-fpm`. xhprof on prints https://<first hostname>:<XHGuiHTTPS>.

Image changes (found by checking the built image)
- conf.d was root-owned: Dockerfile now `chown www:www /usr/local/etc/php/conf.d`.
- pkill missing: added `procps` apt package.
- image.sh gained a check for pkill and conf.d writability. `images/php/test/image.sh 8.5` PASS.
- Smoke in temp container (removed after): toggle as uid 1000 renamed xdebug.ini.off -> xdebug.ini, fpm master (pid 778) survived USR2, second run no-op exit 0.

Not covered: -s given after a non-passthrough command is handled by cobra; real tty path not exercised in tests.
