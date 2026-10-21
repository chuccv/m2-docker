package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/chuccv/magento2-docker/internal/config"
	"github.com/chuccv/magento2-docker/internal/install"
	"github.com/chuccv/magento2-docker/internal/run"
	"github.com/chuccv/magento2-docker/internal/site"
	"github.com/chuccv/magento2-docker/internal/snapshot"
)

const (
	mcpMaxOut      = 20 * 1024
	mcpDefaultLogs = 200
	mcpMaxLogs     = 5000
)

var (
	errNoSite = errors.New("site is required")
	safeName  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

const mcpInstructions = `Setting up a site:
- Blank folder: run "md install" in the CLI.
- Folder with .ddev: run "md migrate" in the CLI.
- Folder with Magento code but no .md/config.yaml: call site_init.
- Never hand-write .md/config.yaml.
- Never report a site as working until its https URL was fetched and answered.
- site_init and site_up output tells you about missing /etc/hosts lines. md never runs sudo here: hand the printed sudo command to the user.`

type quietRunner struct{ run.Runner }

func (q quietRunner) Stream(name string, args ...string) error {
	_, err := q.Run(name, args...)
	return err
}

type mcpServer struct{ e *env }

func (m mcpServer) call(fn func(e *env) error) (string, error) {
	c := *m.e
	var buf bytes.Buffer
	c.out = &buf
	c.errOut = &buf
	c.tty = func() bool { return false }
	c.runner = quietRunner{m.e.runner}
	err := fn(&c)
	return buf.String(), err
}

func (m mcpServer) site(name string) (*env, site.Site, error) {
	if name == "" {
		return nil, site.Site{}, errNoSite
	}
	c := *m.e
	c.runner = quietRunner{m.e.runner}
	s, err := c.findSite(name)
	return &c, s, err
}

func capText(s string, tail bool) string {
	if len(s) <= mcpMaxOut {
		return s
	}
	marker := fmt.Sprintf("\n[truncated: %d bytes total, limit %d]\n", len(s), mcpMaxOut)
	keep := mcpMaxOut - len(marker)
	if tail {
		return marker + strings.ToValidUTF8(s[len(s)-keep:], "")
	}
	return strings.ToValidUTF8(s[:keep], "") + marker
}

func addTool[In any](s *mcp.Server, name, desc string, h func(In) (string, error)) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc}, func(_ context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		out, err := h(in)
		res := &mcp.CallToolResult{}
		if err != nil {
			res.IsError = true
			if out != "" {
				out += "\n"
			}
			out += err.Error()
		}
		res.Content = []mcp.Content{&mcp.TextContent{Text: capText(out, false)}}
		return res, nil, nil
	})
}

type siteIn struct {
	Site string `json:"site" jsonschema:"md site name"`
}

type magentoIn struct {
	Site string   `json:"site" jsonschema:"md site name"`
	Args []string `json:"args" jsonschema:"arguments for php bin/magento"`
}

type execIn struct {
	Site string `json:"site" jsonschema:"md site name"`
	Cmd  string `json:"cmd" jsonschema:"shell command run with bash -lc in the Magento root"`
}

type sqlIn struct {
	Site  string `json:"site" jsonschema:"md site name"`
	Query string `json:"query" jsonschema:"SQL statement, writes allowed"`
}

type logsIn struct {
	Site  string `json:"site" jsonschema:"md site name"`
	File  string `json:"file" jsonschema:"exception, system, debug or report"`
	ID    string `json:"id,omitempty" jsonschema:"report id, required when file is report"`
	Lines int    `json:"lines,omitempty" jsonschema:"last N lines, default 200"`
	Grep  string `json:"grep,omitempty" jsonschema:"only lines containing this fixed string"`
}

type snapshotCreateIn struct {
	Site  string `json:"site" jsonschema:"md site name"`
	Label string `json:"label,omitempty" jsonschema:"snapshot label, default timestamp"`
}

type snapshotRestoreIn struct {
	Site string `json:"site" jsonschema:"md site name"`
	Name string `json:"name" jsonschema:"snapshot file name from snapshot_list"`
}

type initIn struct {
	Dir          string `json:"dir" jsonschema:"absolute path of the folder that already has Magento code"`
	Version      string `json:"version,omitempty" jsonschema:"Magento version, default from composer.lock"`
	PHP          string `json:"php,omitempty" jsonschema:"PHP version override"`
	DB           string `json:"db,omitempty" jsonschema:"database version override"`
	DBType       string `json:"db_type,omitempty" jsonschema:"mariadb or mysql"`
	Search       string `json:"search,omitempty" jsonschema:"search engine version override"`
	SearchEngine string `json:"search_engine,omitempty" jsonschema:"opensearch or elasticsearch, needs search"`
	Varnish      bool   `json:"varnish,omitempty" jsonschema:"add Varnish"`
}

type toggleIn struct {
	Site string `json:"site" jsonschema:"md site name"`
	On   bool   `json:"on" jsonschema:"true to enable, false to disable"`
}

func (m mcpServer) sitesList(_ struct{}) (string, error) {
	return m.call(func(e *env) error {
		home, err := e.home()
		if err != nil {
			return err
		}
		cfg, err := config.Load(home)
		if err != nil {
			return err
		}
		st, err := loadState(home)
		if err != nil {
			return err
		}
		running, err := e.runningProjects()
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(e.out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tSTATUS\tURL\tROOT")
		for _, ref := range st.Sites {
			status := "stopped"
			if running["md-"+ref.Name] {
				status = "running"
			}
			url := "-"
			if s, err := site.Load(ref.Root); err == nil && len(s.Hostnames) > 0 {
				url = siteURL(s.Hostnames[0], cfg.Ports.HTTPS)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", ref.Name, status, url, ref.Root)
		}
		return tw.Flush()
	})
}

func (m mcpServer) simple(in siteIn, fn func(e *env, name string) error) (string, error) {
	if in.Site == "" {
		return "", errNoSite
	}
	return m.call(func(e *env) error { return fn(e, in.Site) })
}

func (m mcpServer) describe(in siteIn) (string, error) {
	return m.simple(in, func(e *env, n string) error { return e.describe(n) })
}

func (m mcpServer) up(in siteIn) (string, error) {
	return m.simple(in, func(e *env, n string) error { return e.upByName(n) })
}

func (m mcpServer) stop(in siteIn) (string, error) {
	return m.simple(in, func(e *env, n string) error { return e.stopByName(n) })
}

func (m mcpServer) restart(in siteIn) (string, error) {
	return m.simple(in, func(e *env, n string) error { return e.restartByName(n) })
}

func (m mcpServer) siteInit(in initIn) (string, error) {
	if in.Dir == "" {
		return "", errors.New("dir is required")
	}
	if !filepath.IsAbs(in.Dir) {
		return "", errors.New("dir must be an absolute path")
	}
	return m.call(func(e *env) error {
		return e.initSite(install.InitOptions{Root: in.Dir, Version: in.Version, PHP: in.PHP, DB: in.DB, DBType: in.DBType, Search: in.Search, SearchEngine: in.SearchEngine, Varnish: in.Varnish})
	})
}

func (m mcpServer) execWeb(name string, tail ...string) (string, error) {
	e, s, err := m.site(name)
	if err != nil {
		return "", err
	}
	args := append([]string{"exec", "-u", containerUID, "-w", webRoot, "ddev-" + s.Name + "-web"}, tail...)
	return e.runner.Run("docker", args...)
}

func (m mcpServer) magento(in magentoIn) (string, error) {
	return m.execWeb(in.Site, append([]string{"php", "bin/magento"}, in.Args...)...)
}

func (m mcpServer) exec(in execIn) (string, error) {
	if strings.TrimSpace(in.Cmd) == "" {
		return "", errNoCommand
	}
	return m.execWeb(in.Site, "bash", "-lc", in.Cmd)
}

func (m mcpServer) sql(in sqlIn) (string, error) {
	if strings.TrimSpace(in.Query) == "" {
		return "", errors.New("query is required")
	}
	e, s, err := m.site(in.Site)
	if err != nil {
		return "", err
	}
	return e.runner.Run("docker", "exec", "ddev-"+s.Name+"-db", s.DBClient(), "-udb", "-pdb", "db", "-B", "-e", in.Query)
}

func (m mcpServer) logs(in logsIn) (string, error) {
	var out string
	var err error
	switch in.File {
	case "exception", "system", "debug":
		n := in.Lines
		if n <= 0 {
			n = mcpDefaultLogs
		}
		if n > mcpMaxLogs {
			n = mcpMaxLogs
		}
		script := `tail -n "$1" "$2"`
		args := []string{"sh", "-c", script, "sh", strconv.Itoa(n), "var/log/" + in.File + ".log"}
		if in.Grep != "" {
			args[2] = `grep -F -e "$3" "$2" | tail -n "$1"`
			args = append(args, in.Grep)
		}
		out, err = m.execWeb(in.Site, args...)
	case "report":
		if !safeName.MatchString(in.ID) {
			return "", errors.New("report needs a valid id")
		}
		out, err = m.execWeb(in.Site, "cat", "var/report/"+in.ID)
	default:
		return "", errors.New("file must be exception, system, debug or report")
	}
	return capText(out, true), err
}

func (m mcpServer) snapshotCreate(in snapshotCreateIn) (string, error) {
	if in.Label != "" && !safeName.MatchString(in.Label) {
		return "", errors.New("label may only contain letters, digits, dot, dash and underscore")
	}
	e, s, err := m.site(in.Site)
	if err != nil {
		return "", err
	}
	home, err := e.home()
	if err != nil {
		return "", err
	}
	p, err := snapshot.Create(home, s, e.runner, in.Label)
	return filepath.Base(p), err
}

func (m mcpServer) snapshotList(in siteIn) (string, error) {
	e, s, err := m.site(in.Site)
	if err != nil {
		return "", err
	}
	home, err := e.home()
	if err != nil {
		return "", err
	}
	files, err := snapshot.List(home, s)
	var names []string
	for _, f := range files {
		names = append(names, filepath.Base(f))
	}
	return strings.Join(names, "\n"), err
}

func (m mcpServer) snapshotRestore(in snapshotRestoreIn) (string, error) {
	e, s, err := m.site(in.Site)
	if err != nil {
		return "", err
	}
	home, err := e.home()
	if err != nil {
		return "", err
	}
	files, err := snapshot.List(home, s)
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if filepath.Base(f) == in.Name {
			if err := snapshot.Restore(home, s, e.runner, f); err != nil {
				return "", err
			}
			return "restored " + in.Name, nil
		}
	}
	return "", fmt.Errorf("snapshot %q not found for %s", in.Name, s.Name)
}

func (m mcpServer) toggle(ext string) func(toggleIn) (string, error) {
	return func(in toggleIn) (string, error) {
		out, err := m.simple(siteIn{in.Site}, func(e *env, n string) error { return e.toggle(n, ext, in.On) })
		if err == nil && out == "" {
			out = "ok"
		}
		return out, err
	}
}

func (m mcpServer) cron(in toggleIn) (string, error) {
	return m.simple(siteIn{in.Site}, func(e *env, n string) error { return e.cron(n, in.On) })
}

func (m mcpServer) build() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "md", Version: "1"}, &mcp.ServerOptions{Instructions: mcpInstructions})
	addTool(s, "sites_list", "List md sites with status, URL and root", m.sitesList)
	addTool(s, "site_describe", "Show containers, hostnames, database credentials and ports of a site", m.describe)
	addTool(s, "site_init", "Set up a folder that already has Magento code but no .md/config.yaml: detects version and edition from composer.lock, writes .md/config.yaml, starts the site. Never needs sudo; reports missing /etc/hosts lines", m.siteInit)
	addTool(s, "site_up", "Start a site", m.up)
	addTool(s, "site_stop", "Stop a site, data volumes are kept", m.stop)
	addTool(s, "site_restart", "Stop then start a site", m.restart)
	addTool(s, "magento", "Run php bin/magento in the site web container", m.magento)
	addTool(s, "exec", "Run a shell command in the site web container, working dir is the Magento root", m.exec)
	addTool(s, "sql", "Run SQL against the site database, output is tab separated", m.sql)
	addTool(s, "logs", "Read the tail of var/log/{exception,system,debug}.log or a var/report/<id> file", m.logs)
	addTool(s, "snapshot_create", "Create a database snapshot", m.snapshotCreate)
	addTool(s, "snapshot_list", "List database snapshots, newest first", m.snapshotList)
	addTool(s, "snapshot_restore", "Restore a snapshot into the site database, a pre-restore snapshot is taken first", m.snapshotRestore)
	addTool(s, "xdebug", "Turn xdebug on or off in the web container", m.toggle("xdebug"))
	addTool(s, "xhprof", "Turn xhprof on or off in the web container, on returns the XHGui URL", m.toggle("xhprof"))
	addTool(s, "cron", "Turn Magento cron (supercronic) on or off for a site; persisted in .md/config.yaml", m.cron)
	return s
}

func newMCPCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve an MCP server over stdio for AI assistants",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			return mcpServer{e}.build().Run(ctx, &mcp.StdioTransport{})
		},
	}
}
