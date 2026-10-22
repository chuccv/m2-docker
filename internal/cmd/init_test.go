package cmd

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chuccv/magento2-docker/internal/run"
	"github.com/chuccv/magento2-docker/internal/site"
)

func initSiteFixture(t *testing.T, tty bool) (*env, *run.FakeRunner, *strings.Builder, *strings.Builder, site.Site) {
	t.Helper()
	e, f, root := testEnv(t, tty)
	out, errOut := &strings.Builder{}, &strings.Builder{}
	e.out, e.errOut = out, errOut
	e.lookup = func(string) ([]string, error) { return nil, errors.New("nxdomain") }
	s, err := site.Find("ee249-qa", "", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	return e, f, out, errOut, s
}

func TestCheckDomainNonTTYReturnsCommand(t *testing.T) {
	e, f, _, _, s := initSiteFixture(t, false)
	e.httpGet = func(string) (int, string, error) { t.Fatal("must not probe"); return 0, "", nil }
	err := e.checkDomain(s, 443)
	if err == nil || !strings.Contains(err.Error(), "md hosts ee249-qa | sudo tee -a /etc/hosts") {
		t.Fatalf("err %v", err)
	}
	if len(f.Calls) != 0 {
		t.Fatalf("calls %v", f.Calls)
	}
}

func TestCheckDomainTTYRunsSudoThenProbes(t *testing.T) {
	e, f, out, _, s := initSiteFixture(t, true)
	var got string
	e.httpGet = func(u string) (int, string, error) { got = u; return 302, "/setup/", nil }
	if err := e.checkDomain(s, 443); err != nil {
		t.Fatal(err)
	}
	if string(f.Stdins["sudo tee -a /etc/hosts"]) != "127.0.0.1 ee249-qa.md.site\n127.0.0.1 shop.md.site\n" {
		t.Fatalf("stdins %q", f.Stdins)
	}
	if got != "https://ee249-qa.md.site/" || !strings.Contains(out.String(), "https://ee249-qa.md.site/ → 302 /setup/") {
		t.Fatalf("url %q out %q", got, out.String())
	}
}

func TestCheckDomainResolvedSkipsSudo(t *testing.T) {
	e, f, out, _, s := initSiteFixture(t, false)
	e.lookup = func(string) ([]string, error) { return []string{"127.0.0.1"}, nil }
	e.httpGet = func(string) (int, string, error) { return 200, "", nil }
	if err := e.checkDomain(s, 443); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls) != 0 || !strings.Contains(out.String(), "→ 200") {
		t.Fatalf("calls %v out %q", f.Calls, out.String())
	}
}

func TestCheckDomainProbeError(t *testing.T) {
	e, _, _, _, s := initSiteFixture(t, false)
	e.lookup = func(string) ([]string, error) { return []string{"127.0.0.1"}, nil }
	e.httpGet = func(string) (int, string, error) { return 0, "", errors.New("x509") }
	if err := e.checkDomain(s, 443); err == nil || !strings.Contains(err.Error(), "x509") {
		t.Fatalf("err %v", err)
	}
}

func TestWarnHosts(t *testing.T) {
	e, _, _, errOut, s := initSiteFixture(t, false)
	e.warnHosts(s)
	if !strings.Contains(errOut.String(), "md hosts ee249-qa | sudo tee -a /etc/hosts") {
		t.Fatalf("stderr %q", errOut.String())
	}
	errOut.Reset()
	e.lookup = func(string) ([]string, error) { return []string{"127.0.0.1"}, nil }
	e.warnHosts(s)
	if errOut.Len() != 0 {
		t.Fatalf("stderr %q", errOut.String())
	}
}

func TestInitSteps(t *testing.T) {
	e, _, out, _, s := initSiteFixture(t, false)
	s.Root = t.TempDir()
	e.printInitSteps(s, 443)
	if !strings.Contains(out.String(), "md mysql") || !strings.Contains(out.String(), "setup:upgrade") {
		t.Fatalf("out %q", out.String())
	}
	out.Reset()
	writeFile(filepath.Join(s.Root, "app", "etc", "env.php"), []byte("<?php"))
	e.printInitSteps(s, 443)
	if !strings.Contains(out.String(), "--db-user=db") {
		t.Fatalf("out %q", out.String())
	}
}
