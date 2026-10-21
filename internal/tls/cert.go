package tls

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chuccv/magento2-docker/internal/run"
	"github.com/chuccv/magento2-docker/internal/site"
)

const renewWindow = 30 * 24 * time.Hour

func EnsureCert(s site.Site, home string, r run.Runner) (crt, key string, err error) {
	dir := filepath.Join(home, "certs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	crt = filepath.Join(dir, s.Name+".crt")
	key = filepath.Join(dir, s.Name+".key")
	if valid(crt, s.Hostnames) {
		if _, err := os.Stat(key); err == nil {
			return crt, key, nil
		}
	}
	args := append([]string{"-cert-file", crt, "-key-file", key}, s.Hostnames...)
	if _, err := r.Run("mkcert", args...); err != nil {
		return "", "", fmt.Errorf("mkcert: %w", err)
	}
	return crt, key, nil
}

func valid(path string, hosts []string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return false
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	if time.Until(c.NotAfter) < renewWindow {
		return false
	}
	for _, h := range hosts {
		if c.VerifyHostname(h) != nil {
			return false
		}
	}
	return true
}

func EnsureDefaultCert(home string, r run.Runner) (crt, key string, err error) {
	hosts := []string{"*." + site.Domain, site.Domain}
	if site.Domain != site.LegacyDomain {
		hosts = append(hosts, "*."+site.LegacyDomain, site.LegacyDomain)
	}
	return EnsureCert(site.Site{Name: "_default", Hostnames: append(hosts, "*.ddev.site", "ddev.site")}, home, r)
}
