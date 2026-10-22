package cmd

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const releaseAPI = "https://api.github.com/repos/chuccv/magento2-docker/releases/latest"

func newUpdateCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update md to the latest GitHub release",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			if exe, err = filepath.EvalSymlinks(exe); err != nil {
				return err
			}
			return selfUpdate(releaseAPI, c.Root().Version, exe, e.writer())
		},
	}
}

func fetch(url string) ([]byte, error) {
	c := &http.Client{Timeout: 5 * time.Minute}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func selfUpdate(api, current, exe string, out io.Writer) error {
	if current == "dev" || current == "" {
		return errors.New("md was built from source (version dev): update it with go build or go install")
	}
	body, err := fetch(api)
	if err != nil {
		return err
	}
	var rel struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return fmt.Errorf("parse %s: %w", api, err)
	}
	if rel.Tag == current {
		fmt.Fprintf(out, "md %s is up to date\n", current)
		return nil
	}
	name := "md-" + runtime.GOOS + "-" + runtime.GOARCH
	urls := map[string]string{}
	for _, a := range rel.Assets {
		urls[a.Name] = a.URL
	}
	if urls[name] == "" || urls["SHA256SUMS"] == "" {
		return fmt.Errorf("release %s has no %s or SHA256SUMS", rel.Tag, name)
	}
	sums, err := fetch(urls["SHA256SUMS"])
	if err != nil {
		return err
	}
	want := ""
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) == 2 && f[1] == name {
			want = f[0]
		}
	}
	if want == "" {
		return fmt.Errorf("SHA256SUMS of %s has no %s", rel.Tag, name)
	}
	bin, err := fetch(urls[name])
	if err != nil {
		return err
	}
	if sum := sha256.Sum256(bin); hex.EncodeToString(sum[:]) != want {
		return fmt.Errorf("%s checksum mismatch, %s not changed", name, exe)
	}
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".md-update-*")
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("no write access to %s: run `sudo md update`", filepath.Dir(exe))
	}
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), exe); err != nil {
		return err
	}
	fmt.Fprintf(out, "md %s → %s (%s). Restart `md ui` if it is running.\n", current, rel.Tag, exe)
	return nil
}
