package cmd

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/chuccv/magento2-docker/internal/run"
)

func mcpSession(t *testing.T, e *env) *mcp.ClientSession {
	t.Helper()
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := mcpServer{e}.build().Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		sb.WriteString(c.(*mcp.TextContent).Text)
	}
	return sb.String(), res.IsError
}

func lastCall(f *run.FakeRunner) []string {
	return f.Calls[len(f.Calls)-1]
}

func TestMCPToolList(t *testing.T) {
	e, _, _ := testEnv(t, false)
	cs := mcpSession(t, e)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tool := range res.Tools {
		got = append(got, tool.Name)
	}
	sort.Strings(got)
	want := []string{"exec", "logs", "magento", "site_describe", "site_init", "site_restart", "site_stop", "site_up", "sites_list", "snapshot_create", "snapshot_list", "snapshot_restore", "sql", "xdebug", "xhprof"}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tools %v\nwant %v", got, want)
	}
}

func TestMCPMagento(t *testing.T) {
	e, f, _ := testEnv(t, false)
	f.Out = map[string]string{"docker exec -u 1000 -w /var/www/html ddev-ee249-qa-web php bin/magento cache:flush": "flushed\n"}
	out, isErr := callTool(t, mcpSession(t, e), "magento", map[string]any{"site": "ee249-qa", "args": []string{"cache:flush"}})
	if isErr || out != "flushed\n" {
		t.Fatalf("got %q err=%v", out, isErr)
	}
	want(t, f, "docker", "exec", "-u", "1000", "-w", "/var/www/html", "ddev-ee249-qa-web", "php", "bin/magento", "cache:flush")
}

func TestMCPExec(t *testing.T) {
	e, f, _ := testEnv(t, false)
	callTool(t, mcpSession(t, e), "exec", map[string]any{"site": "ee249-qa", "cmd": "ls | wc -l"})
	want(t, f, "docker", "exec", "-u", "1000", "-w", "/var/www/html", "ddev-ee249-qa-web", "bash", "-lc", "ls | wc -l")
}

func TestMCPSQL(t *testing.T) {
	e, f, _ := testEnv(t, false)
	callTool(t, mcpSession(t, e), "sql", map[string]any{"site": "ee249-qa", "query": "UPDATE x SET a=1"})
	want(t, f, "docker", "exec", "ddev-ee249-qa-db", "mariadb", "-udb", "-pdb", "db", "-B", "-e", "UPDATE x SET a=1")
}

func TestMCPLogs(t *testing.T) {
	e, f, _ := testEnv(t, false)
	cs := mcpSession(t, e)
	base := []string{"docker", "exec", "-u", "1000", "-w", "/var/www/html", "ddev-ee249-qa-web", "sh", "-c"}

	callTool(t, cs, "logs", map[string]any{"site": "ee249-qa", "file": "exception"})
	want(t, f, append(base, `tail -n "$1" "$2"`, "sh", "200", "var/log/exception.log")...)

	f.Calls = nil
	callTool(t, cs, "logs", map[string]any{"site": "ee249-qa", "file": "system", "lines": 50, "grep": "Foo"})
	want(t, f, append(base, `grep -F -e "$3" "$2" | tail -n "$1"`, "sh", "50", "var/log/system.log", "Foo")...)

	f.Calls = nil
	callTool(t, cs, "logs", map[string]any{"site": "ee249-qa", "file": "report", "id": "123abc"})
	want(t, f, "docker", "exec", "-u", "1000", "-w", "/var/www/html", "ddev-ee249-qa-web", "cat", "var/report/123abc")
}

func TestMCPLogsRejectsBadInput(t *testing.T) {
	e, f, _ := testEnv(t, false)
	cs := mcpSession(t, e)
	for _, args := range []map[string]any{
		{"site": "ee249-qa", "file": "report", "id": "../../etc/passwd"},
		{"site": "ee249-qa", "file": "report"},
		{"site": "ee249-qa", "file": "../env"},
	} {
		if _, isErr := callTool(t, cs, "logs", args); !isErr {
			t.Fatalf("want error for %v", args)
		}
	}
	if len(f.Calls) != 0 {
		t.Fatalf("calls %v", f.Calls)
	}
}

func TestMCPUnknownSite(t *testing.T) {
	e, f, _ := testEnv(t, false)
	cs := mcpSession(t, e)
	out, isErr := callTool(t, cs, "magento", map[string]any{"site": "nope", "args": []string{"x"}})
	if !isErr || !strings.Contains(out, "not found") {
		t.Fatalf("got %q err=%v", out, isErr)
	}
	if _, isErr := callTool(t, cs, "site_up", map[string]any{"site": ""}); !isErr {
		t.Fatal("empty site must fail")
	}
	if len(f.Calls) != 0 {
		t.Fatalf("calls %v", f.Calls)
	}
}

func TestMCPErrorIncludesStderrAndStdout(t *testing.T) {
	e, f, _ := testEnv(t, false)
	key := "docker exec -u 1000 -w /var/www/html ddev-ee249-qa-web php bin/magento boom"
	f.Out = map[string]string{key: "partial"}
	f.Err = map[string]error{key: errors.New("exit status 1: Command boom is not defined")}
	out, isErr := callTool(t, mcpSession(t, e), "magento", map[string]any{"site": "ee249-qa", "args": []string{"boom"}})
	if !isErr || !strings.Contains(out, "partial") || !strings.Contains(out, "is not defined") {
		t.Fatalf("got %q err=%v", out, isErr)
	}
}

func TestMCPTruncation(t *testing.T) {
	e, f, _ := testEnv(t, false)
	f.Out = map[string]string{"docker exec -u 1000 -w /var/www/html ddev-ee249-qa-web bash -lc big": strings.Repeat("a", 100000)}
	out, isErr := callTool(t, mcpSession(t, e), "exec", map[string]any{"site": "ee249-qa", "cmd": "big"})
	if isErr || len(out) > mcpMaxOut || !strings.Contains(out, "[truncated: 100000 bytes total") {
		t.Fatalf("len=%d err=%v", len(out), isErr)
	}
	if got := capText(strings.Repeat("a", 100)+"END", true); got != strings.Repeat("a", 100)+"END" {
		t.Fatal("short text must be untouched")
	}
	tail := capText(strings.Repeat("a", 30000)+"END", true)
	if len(tail) > mcpMaxOut || !strings.HasSuffix(tail, "END") || !strings.HasPrefix(tail, "\n[truncated") {
		t.Fatalf("tail truncation wrong: len=%d", len(tail))
	}
}

func TestMCPXhprofOnReturnsURL(t *testing.T) {
	e, f, _ := testEnv(t, false)
	out, isErr := callTool(t, mcpSession(t, e), "xhprof", map[string]any{"site": "ee249-qa", "on": true})
	if isErr || !strings.Contains(out, "https://ee249-qa.md.site:8142") {
		t.Fatalf("got %q err=%v", out, isErr)
	}
	want(t, f, "docker", "exec", "-u", "1000", "ddev-ee249-qa-web", "sh", "-c", toggleScript("xhprof", true))
}

func TestMCPXdebugOff(t *testing.T) {
	e, f, _ := testEnv(t, false)
	callTool(t, mcpSession(t, e), "xdebug", map[string]any{"site": "ee249-qa", "on": false})
	want(t, f, "docker", "exec", "-u", "1000", "ddev-ee249-qa-web", "sh", "-c", toggleScript("xdebug", false))
}

func TestMCPSnapshots(t *testing.T) {
	e, f, _ := testEnv(t, false)
	cs := mcpSession(t, e)
	out, isErr := callTool(t, cs, "snapshot_create", map[string]any{"site": "ee249-qa", "label": "a1"})
	if isErr || out != "ee249-qa-a1.sql.gz" {
		t.Fatalf("got %q err=%v", out, isErr)
	}
	if _, isErr := callTool(t, cs, "snapshot_create", map[string]any{"site": "ee249-qa", "label": "../x"}); !isErr {
		t.Fatal("bad label must fail")
	}
	out, _ = callTool(t, cs, "snapshot_list", map[string]any{"site": "ee249-qa"})
	if out != "ee249-qa-a1.sql.gz" {
		t.Fatalf("list %q", out)
	}
	f.Calls = nil
	if _, isErr := callTool(t, cs, "snapshot_restore", map[string]any{"site": "ee249-qa", "name": "../../etc/passwd"}); !isErr {
		t.Fatal("unknown snapshot must fail")
	}
	if len(f.Calls) != 0 {
		t.Fatalf("calls %v", f.Calls)
	}
	out, isErr = callTool(t, cs, "snapshot_restore", map[string]any{"site": "ee249-qa", "name": "ee249-qa-a1.sql.gz"})
	if isErr || out != "restored ee249-qa-a1.sql.gz" {
		t.Fatalf("got %q err=%v", out, isErr)
	}
	if got := lastCall(f); !reflect.DeepEqual(got, []string{"docker", "exec", "-i", "ddev-ee249-qa-db", "mariadb", "-udb", "-pdb", "db"}) {
		t.Fatalf("last call %v", got)
	}
}

func TestMCPSitesListAndStop(t *testing.T) {
	e, f, root := testEnv(t, false)
	home, _ := e.home()
	if err := saveState(home, state{Sites: []siteRef{{Name: "ee249-qa", Root: root + "/ee249-qa"}}}); err != nil {
		t.Fatal(err)
	}
	f.Out = map[string]string{"docker compose ls --format json": `[{"Name":"md-ee249-qa","Status":"running(3)"}]`}
	cs := mcpSession(t, e)
	out, isErr := callTool(t, cs, "sites_list", map[string]any{})
	if isErr || !strings.Contains(out, "ee249-qa") || !strings.Contains(out, "running") || !strings.Contains(out, "https://ee249-qa.md.site") || !strings.Contains(out, root+"/ee249-qa") {
		t.Fatalf("got %q err=%v", out, isErr)
	}
	f.Calls = nil
	if _, isErr := callTool(t, cs, "site_stop", map[string]any{"site": "ee249-qa"}); isErr {
		t.Fatal("stop failed")
	}
	if got := lastCall(f); got[1] != "compose" || got[len(got)-1] != "down" {
		t.Fatalf("calls %v", f.Calls)
	}
}
