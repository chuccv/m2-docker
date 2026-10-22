package clean

import (
	"reflect"
	"testing"

	"github.com/chuccv/magento2-docker/internal/run"
	"github.com/chuccv/magento2-docker/internal/site"
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

func TestPlanSkipsProtectedVolumesAndXhguiImage(t *testing.T) {
	r := fake()
	r.Out["docker volume ls -f dangling=true -q"] = "md-opensearch-latest-data\nmd-opensearch-latest-plugins\nmd-elasticsearch-7.17.14-data\nmd-elasticsearch-7.17.14-plugins\nshop-a-mysql\nmd-node-22\nmd-xhgui-data\nddev-ee249-ln_redis\nx-mariadb\nsome-other-volume\n"
	r.Out["docker images --format {{.Repository}}:{{.Tag}}\t{{.Size}}"] = "ddev/ddev-xhgui:v1.25.2\t50MB\nddev/ddev-dbserver:v1\t300MB\n"
	items, err := Plan(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Kind == "volume" && it.Name != "some-other-volume" {
			t.Fatalf("protected volume planned: %v", it)
		}
		if it.Name == "ddev/ddev-xhgui:v1.25.2" {
			t.Fatalf("xhgui image planned")
		}
	}
	var vols int
	for _, it := range items {
		if it.Kind == "volume" {
			vols++
		}
	}
	if vols != 1 {
		t.Fatalf("got %v", items)
	}
}

func TestApplyNeverDeletesProtected(t *testing.T) {
	r := &run.FakeRunner{}
	err := Apply([]Item{
		{"volume", "md-opensearch-latest-data", ""},
		{"volume", "md-node-22", ""},
		{"volume", "ddev-ee249-ln_redis", ""},
		{"volume", "shop-redis", ""},
		{"volume", "x-mariadb", ""},
		{"image", "ddev/ddev-xhgui:v1.25.2", ""},
		{"volume", "some-other-volume", ""},
	}, r)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"docker", "volume", "rm", "some-other-volume"}}
	if !reflect.DeepEqual(r.Calls, want) {
		t.Fatalf("calls %v", r.Calls)
	}
}
