package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runAddon(t *testing.T, envPHP string) ([]string, string) {
	t.Helper()
	e, f, home := upEnv(t)
	dir, _ := e.getwd()
	if envPHP != "" {
		os.MkdirAll(filepath.Join(dir, "app", "etc"), 0o755)
		os.WriteFile(filepath.Join(dir, "app", "etc", "env.php"), []byte(envPHP), 0o644)
	}
	if err := exec(t, e, "addon", "rabbitmq"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, ".md", "config.yaml"))
	compose, _ := os.ReadFile(filepath.Join(home, "sites", "ee249-qa", "compose.yaml"))
	if !strings.Contains(string(cfg), "rabbitmq: true") || !strings.Contains(string(compose), "ddev-ee249-qa-rabbitmq") {
		t.Fatalf("config:\n%s\ncompose:\n%s", cfg, compose)
	}
	return calls(f), e.out.(*strings.Builder).String()
}

func TestAddonRabbitMQSetsAMQP(t *testing.T) {
	cs, out := runAddon(t, "<?php\nreturn [];\n")
	want := "php bin/magento setup:config:set --no-interaction --amqp-host=rabbitmq --amqp-port=5672 --amqp-user=rabbitmq --amqp-password=rabbitmq --amqp-virtualhost=/"
	if index(cs, want) < 0 || !strings.Contains(out, "user rabbitmq") {
		t.Fatalf("calls %v", cs)
	}
}

func TestAddonRabbitMQKeepsEnvAMQP(t *testing.T) {
	cs, out := runAddon(t, "<?php\nreturn ['queue' => ['amqp' => ['host' => 'rabbitmq', 'user' => 'magento', 'password' => 'magento']]];\n")
	if index(cs, "setup:config:set") >= 0 || !strings.Contains(out, "user magento") {
		t.Fatalf("calls %v out %q", cs, out)
	}
}

func TestAddonRabbitMQNotInstalled(t *testing.T) {
	cs, out := runAddon(t, "")
	if index(cs, "setup:config:set") >= 0 || !strings.Contains(out, "add --amqp-host=rabbitmq") {
		t.Fatalf("calls %v out %q", cs, out)
	}
}
