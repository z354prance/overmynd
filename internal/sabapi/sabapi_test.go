package sabapi_test

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z354prance/undermynd/internal/config"
	"github.com/z354prance/undermynd/internal/pipeline"
	"github.com/z354prance/undermynd/internal/sabapi"
	"github.com/z354prance/undermynd/internal/store"
)

// Sonarr works out where finished downloads land from get_config; an empty
// complete_dir makes its health check complain about an invalid path.
func TestGetConfigReportsCompleteFolderAndCategoryDirs(t *testing.T) {
	dir := t.TempDir()
	env := config.Env{ConfigDir: filepath.Join(dir, "c"), DataDir: filepath.Join(dir, "d"), CompleteDir: "/data/Complete"}
	cfg, err := config.Load(env)
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Get()
	s.Categories = []config.Category{{Name: "tv", Kind: "sonarr"}}
	s.ClientPassword = "testkey"
	_ = cfg.Set(s)
	st, _ := store.Open(filepath.Join(env.ConfigDir, "jobs.json"))
	l := log.New(io.Discard, "", 0)
	a := sabapi.New(st, pipeline.New(st, cfg, l), cfg, l)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api?mode=get_config&output=json&apikey=" + "testkey")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	body := string(b)
	if !strings.Contains(body, `"complete_dir":"/data/Complete"`) {
		t.Fatalf("complete_dir missing: %s", body)
	}
	if !strings.Contains(body, `"name":"tv"`) || !strings.Contains(body, `"dir":"tv"`) {
		t.Fatalf("category dir missing: %s", body)
	}
}
