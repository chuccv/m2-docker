package tls

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"m2-docker/internal/run"
)

const remoteCA = "/tmp/md-rootCA.pem"

func Trust(home, remote string, r run.Runner) (string, error) {
	if _, err := r.Run("mkcert", "-install"); err != nil {
		return "", fmt.Errorf("mkcert -install: %w", err)
	}
	out, err := r.Run("mkcert", "-CAROOT")
	if err != nil {
		return "", fmt.Errorf("mkcert -CAROOT: %w", err)
	}
	src := filepath.Join(strings.TrimSpace(out), "rootCA.pem")
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "ca")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "rootCA.pem"), data, 0o644); err != nil {
		return "", err
	}
	if remote != "" {
		if _, err := r.Run("scp", src, remote+":"+remoteCA); err != nil {
			return "", fmt.Errorf("scp: %w", err)
		}
		cmd := "sudo cp " + remoteCA + " /usr/local/share/ca-certificates/md.crt && sudo update-ca-certificates && " +
			"certutil -d sql:$HOME/.pki/nssdb -A -t C,, -n md -i " + remoteCA
		if _, err := r.Run("ssh", remote, cmd); err != nil {
			return "", fmt.Errorf("ssh: %w", err)
		}
	}
	return "Playwright: Chromium uses NSS (covered by mkcert -install). For Node clients set NODE_EXTRA_CA_CERTS=" + src, nil
}
