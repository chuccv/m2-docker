package tls

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"m2-docker/internal/run"
)

func caRoot(t *testing.T) (string, *run.FakeRunner) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rootCA.pem"), []byte("PEM"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, &run.FakeRunner{Out: map[string]string{"mkcert -CAROOT": root + "\n"}}
}

func joined(r *run.FakeRunner) []string {
	var l []string
	for _, c := range r.Calls {
		l = append(l, strings.Join(c, " "))
	}
	return l
}

func TestTrustLocal(t *testing.T) {
	root, r := caRoot(t)
	home := t.TempDir()
	hint, err := Trust(home, "", r)
	if err != nil {
		t.Fatal(err)
	}
	calls := joined(r)
	if len(calls) != 2 || calls[0] != "mkcert -install" || calls[1] != "mkcert -CAROOT" {
		t.Fatalf("calls %v", calls)
	}
	b, err := os.ReadFile(filepath.Join(home, "ca", "rootCA.pem"))
	if err != nil || string(b) != "PEM" {
		t.Fatalf("ca copy: %q %v", b, err)
	}
	if !strings.Contains(hint, "NODE_EXTRA_CA_CERTS="+filepath.Join(root, "rootCA.pem")) {
		t.Fatalf("hint %q", hint)
	}
}

func TestTrustRemote(t *testing.T) {
	root, r := caRoot(t)
	if _, err := Trust(t.TempDir(), "user@host", r); err != nil {
		t.Fatal(err)
	}
	calls := joined(r)
	if len(calls) != 4 {
		t.Fatalf("calls %v", calls)
	}
	if calls[2] != "scp "+filepath.Join(root, "rootCA.pem")+" user@host:/tmp/md-rootCA.pem" {
		t.Fatalf("scp %q", calls[2])
	}
	if !strings.HasPrefix(calls[3], "ssh user@host ") ||
		!strings.Contains(calls[3], "sudo update-ca-certificates") ||
		!strings.Contains(calls[3], "certutil -d sql:$HOME/.pki/nssdb -A -t C,, -n md -i /tmp/md-rootCA.pem") {
		t.Fatalf("remote %q", calls[3])
	}
}
