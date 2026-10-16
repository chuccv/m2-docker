package clean

import (
	"reflect"
	"testing"

	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

func fake() *run.FakeRunner {
	return &run.FakeRunner{Out: map[string]string{
		"docker images -f dangling=true --format {{.ID}}\t{{.Size}}": "abc123\t100MB\n",
		"docker images --format {{.Repository}}:{{.Tag}}\t{{.Size}}": "ddev/ddev-webserver:v1\t500MB\nddev/ddev-dbserver:v1\t300MB\nnginx:1\t10MB\n",
		"docker ps -a --format {{.Image}}":                           "ddev/ddev-webserver:v1\n",
		"docker system df --format {{.Type}}\t{{.Reclaimable}}":      "Images\t1GB\nBuild Cache\t2GB\n",
		"docker volume ls -f dangling=true -q":                       "junk-vol\nsite-es-mariadb\nee249-qa-mariadb\n",
	}}
}

func TestPlan(t *testing.T) {
	items, err := Plan(fake(), []site.Site{{Name: "ee249-qa"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Item{
		{"image", "abc123", "100MB"},
		{"image", "ddev/ddev-dbserver:v1", "300MB"},
		{"buildcache", "build cache", "2GB"},
		{"volume", "junk-vol", ""},
		{"db-orphan", "site-es-mariadb", ""},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("got %v", items)
	}
}

func TestApplyNeverDeletesDB(t *testing.T) {
	r := &run.FakeRunner{}
	err := Apply([]Item{
		{"db-orphan", "site-es-mariadb", ""},
		{"volume", "site-es-mariadb", ""},
		{"image", "abc123", ""},
		{"buildcache", "build cache", ""},
		{"volume", "junk-vol", ""},
	}, r)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"docker", "rmi", "abc123"},
		{"docker", "builder", "prune", "-f"},
		{"docker", "volume", "rm", "junk-vol"},
	}
	if !reflect.DeepEqual(r.Calls, want) {
		t.Fatalf("calls %v", r.Calls)
	}
}
