package clean

import (
	"reflect"
	"testing"

	"github.com/chuccv/magento2-docker/internal/run"
	"github.com/chuccv/magento2-docker/internal/site"
)

func fake() *run.FakeRunner {
	return &run.FakeRunner{Out: map[string]string{
		"docker images -f dangling=true --format {{.ID}}\t{{.Size}}\t{{.CreatedSince}}":                  "abc123\t100MB\t2 days ago\ndef456\t50MB\t3 weeks ago\n",
		"docker image inspect -f {{index .Config.Labels \"com.md.image\"}}\t{{json .Config.Env}} abc123": "php\t[\"PATH=/usr/bin\",\"PHP_VERSION=8.3.35\"]\n",
		"docker image inspect -f {{index .Config.Labels \"com.md.image\"}}\t{{json .Config.Env}} def456": "\tnull\n",
		"docker images --format {{.Repository}}:{{.Tag}}\t{{.Size}}":                                     "ddev/ddev-webserver:v1\t500MB\nddev/ddev-dbserver:v1\t300MB\nnginx:1\t10MB\n",
		"docker ps -a --format {{.Image}}":                                                               "ddev/ddev-webserver:v1\n",
		"docker system df --format {{.Type}}\t{{.Reclaimable}}":                                          "Images\t1GB\nBuild Cache\t2GB\n",
		"docker volume ls -f dangling=true -q":                                                           "junk-vol\nsite-es-mariadb\nee249-qa-mariadb\n",
	}}
}

func TestPlan(t *testing.T) {
	items, err := Plan(fake(), []site.Site{{Name: "ee249-qa"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Item{
		{"image", "abc123", "100MB", "old md/php build (PHP 8.3.35), created 2 days ago"},
		{"image", "def456", "50MB", "untagged image, created 3 weeks ago"},
		{"image", "ddev/ddev-dbserver:v1", "300MB", "unused DDEV image"},
		{"buildcache", "build cache", "2GB", "Docker build cache, the next md/php build is slower"},
		{"volume", "junk-vol", "", "volume used by no container"},
		{"db-orphan", "site-es-mariadb", "", "DB volume of no known site"},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("got %v", items)
	}
}

func TestApplyNeverDeletesDB(t *testing.T) {
	r := &run.FakeRunner{}
	err := Apply([]Item{
		{"db-orphan", "site-es-mariadb", "", ""},
		{"volume", "site-es-mariadb", "", ""},
		{"image", "abc123", "", ""},
		{"buildcache", "build cache", "", ""},
		{"volume", "junk-vol", "", ""},
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
		{"volume", "md-opensearch-latest-data", "", ""},
		{"volume", "md-node-22", "", ""},
		{"volume", "ddev-ee249-ln_redis", "", ""},
		{"volume", "shop-redis", "", ""},
		{"volume", "shop-rabbitmq", "", ""},
		{"volume", "x-mariadb", "", ""},
		{"image", "ddev/ddev-xhgui:v1.25.2", "", ""},
		{"volume", "some-other-volume", "", ""},
	}, r)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"docker", "volume", "rm", "some-other-volume"}}
	if !reflect.DeepEqual(r.Calls, want) {
		t.Fatalf("calls %v", r.Calls)
	}
}

func TestStaleSearch(t *testing.T) {
	r := &run.FakeRunner{Out: map[string]string{
		"docker ps -a --filter label=com.docker.compose.project=md-shared --format {{.Names}}\t{{.Image}}": "md-router\ttraefik:v3.3\nmd-opensearch-3.0.0\topensearchproject/opensearch:3.0.0\nmd-opensearch-2.12.0\topensearchproject/opensearch:2.12.0\n",
		"docker volume ls -q --filter label=com.docker.compose.project=md-shared":                          "md-shared_md-opensearch-3.0.0-data\nmd-shared_md-opensearch-2.12.0-data\nmd-shared_md-opensearch-2.12.0-plugins\nmd-shared_md-xhgui-data\n",
	}}
	items, err := staleSearch(r, []site.Site{{Name: "a", SearchVersion: "3.0.0"}, {Name: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, it := range items {
		names = append(names, it.Kind+" "+it.Name)
	}
	want := []string{"container md-opensearch-2.12.0", "search-volume md-shared_md-opensearch-2.12.0-data", "search-volume md-shared_md-opensearch-2.12.0-plugins", "image opensearchproject/opensearch:2.12.0"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("got %v", names)
	}
	a := &run.FakeRunner{}
	if err := Apply(items, a); err != nil {
		t.Fatal(err)
	}
	wantCalls := [][]string{{"docker", "rm", "-f", "md-opensearch-2.12.0"}, {"docker", "volume", "rm", "md-shared_md-opensearch-2.12.0-data"}, {"docker", "volume", "rm", "md-shared_md-opensearch-2.12.0-plugins"}, {"docker", "rmi", "opensearchproject/opensearch:2.12.0"}}
	if !reflect.DeepEqual(a.Calls, wantCalls) {
		t.Fatalf("calls %v", a.Calls)
	}
	for _, sites := range [][]site.Site{nil, {{Name: "a", SearchVersion: "3.0.0"}, {Name: "x", Type: "error: broken"}}} {
		if items, _ := staleSearch(r, sites); len(items) != 0 {
			t.Fatalf("%v: %v", sites, items)
		}
	}
}
