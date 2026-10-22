package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func releaseServer(t *testing.T, tag string, bin []byte, sum string) *httptest.Server {
	t.Helper()
	name := "md-" + runtime.GOOS + "-" + runtime.GOARCH
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":%q,"browser_download_url":"%s/bin"},{"name":"SHA256SUMS","browser_download_url":"%s/sums"}]}`, tag, name, srv.URL, srv.URL)
		case "/bin":
			w.Write(bin)
		case "/sums":
			fmt.Fprintf(w, "%s  md-other-arch\n%s  %s\n", strings.Repeat("0", 64), sum, name)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fakeExe(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "md")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestSelfUpdateReplacesBinary(t *testing.T) {
	bin := []byte("new binary")
	srv := releaseServer(t, "v1.0.6", bin, sha(bin))
	exe := fakeExe(t)
	out := &strings.Builder{}
	if err := selfUpdate(srv.URL+"/latest", "v1.0.5", exe, out); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	fi, _ := os.Stat(exe)
	if string(got) != "new binary" || fi.Mode().Perm() != 0o755 || !strings.Contains(out.String(), "v1.0.5 → v1.0.6") {
		t.Fatalf("got %q mode %v out %q", got, fi.Mode(), out.String())
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".md-update-*")); len(left) != 0 {
		t.Fatalf("temp files left %v", left)
	}
}

func TestSelfUpdateUpToDate(t *testing.T) {
	srv := releaseServer(t, "v1.0.5", []byte("x"), sha([]byte("x")))
	exe := fakeExe(t)
	out := &strings.Builder{}
	if err := selfUpdate(srv.URL+"/latest", "v1.0.5", exe, out); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old" || !strings.Contains(out.String(), "up to date") {
		t.Fatalf("got %q out %q", got, out.String())
	}
}

func TestSelfUpdateChecksumMismatchKeepsBinary(t *testing.T) {
	srv := releaseServer(t, "v1.0.6", []byte("tampered"), sha([]byte("new binary")))
	exe := fakeExe(t)
	err := selfUpdate(srv.URL+"/latest", "v1.0.5", exe, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old" {
		t.Fatalf("binary changed to %q", got)
	}
}

func TestSelfUpdateRefusesDevBuild(t *testing.T) {
	err := selfUpdate("http://127.0.0.1:1/latest", "dev", fakeExe(t), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "built from source") {
		t.Fatalf("err %v", err)
	}
}

func TestSelfUpdateNoWriteAccess(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write anywhere")
	}
	bin := []byte("new binary")
	srv := releaseServer(t, "v1.0.6", bin, sha(bin))
	exe := fakeExe(t)
	dir := filepath.Dir(exe)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	err := selfUpdate(srv.URL+"/latest", "v1.0.5", exe, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "sudo md self-update") {
		t.Fatalf("err %v", err)
	}
}
