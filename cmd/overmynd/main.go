package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // so TZ works in a minimal image

	"github.com/z354prance/overmynd/internal/auth"
	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/logbuf"
	"github.com/z354prance/overmynd/internal/pipeline"
	"github.com/z354prance/overmynd/internal/progress"
	"github.com/z354prance/overmynd/internal/qbitapi"
	"github.com/z354prance/overmynd/internal/rss"
	"github.com/z354prance/overmynd/internal/sabapi"
	"github.com/z354prance/overmynd/internal/storage"
	"github.com/z354prance/overmynd/internal/store"
	"github.com/z354prance/overmynd/internal/watch"
	"github.com/z354prance/overmynd/internal/web"
)

// dropPrivileges runs the app as PUID:PGID (Unraid: 99:100) when started as root,
// after making sure it owns its config and data folders.
func dropPrivileges(logger *log.Logger, env config.Env) {
	if os.Getuid() != 0 {
		return
	}
	uid, err1 := strconv.Atoi(os.Getenv("PUID"))
	gid, err2 := strconv.Atoi(os.Getenv("PGID"))
	if err1 != nil || err2 != nil || uid == 0 {
		return
	}
	for _, d := range []string{env.ConfigDir, env.DataDir, env.Work(), env.Complete()} {
		if _, err := os.Stat(d); err == nil {
			continue // never change the owner of a folder that already exists
		}
		if err := os.MkdirAll(d, 0o755); err == nil {
			_ = os.Chown(d, uid, gid)
		}
	}
	// Chown the top level of the config folder (settings, keys, job file) in case an earlier run was root.
	if ents, err := os.ReadDir(env.ConfigDir); err == nil {
		for _, e := range ents {
			_ = os.Chown(filepath.Join(env.ConfigDir, e.Name()), uid, gid)
		}
	}
	if err := syscall.Setgid(gid); err != nil {
		logger.Fatalf("setgid: %v", err)
	}
	if err := syscall.Setuid(uid); err != nil {
		logger.Fatalf("setuid: %v", err)
	}
	syscall.Umask(0o002) // group-writable, so the arrs and Tdarr can use what we create
	logger.Printf("running as %d:%d", uid, gid)
}

// healthcheck is used by the container HEALTHCHECK (the image has no curl).
func healthcheck(addr string) {
	if len(addr) > 0 && addr[0] == ':' {
		addr = "127.0.0.1" + addr
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + addr + "/health")
	if err != nil || resp.StatusCode != 200 {
		os.Exit(1)
	}
}

func main() {
	logs := logbuf.New(2000)
	logger := log.New(io.MultiWriter(os.Stdout, logs), "overmynd ", log.LstdFlags)
	env := config.LoadEnv()
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		healthcheck(env.PublicAddr)
		return
	}
	dropPrivileges(logger, env)
	cfg, err := config.Load(env)
	if err != nil {
		logger.Fatalf("config: %v", err)
	}
	st, err := store.Open(filepath.Join(env.ConfigDir, "jobs.json"))
	if err != nil {
		logger.Fatalf("store: %v", err)
	}
	au, err := auth.New(env.ConfigDir)
	if err != nil {
		logger.Fatalf("auth: %v", err)
	}
	eng := pipeline.New(st, cfg, logger)
	qb := qbitapi.New(st, eng, cfg, logger)
	sab := sabapi.New(st, eng, cfg, logger)
	machine := http.NewServeMux()
	machine.Handle("/api", sab.Handler())
	machine.Handle("/sabnzbd/", sab.Handler())
	machine.Handle("/", qb.Handler())
	site := web.New(st, eng, cfg, au, logger)
	site.Logs = logs

	if am, err := web.NewAccess(env.ConfigDir, cfg); err != nil {
		logger.Fatalf("access: %v", err)
	} else {
		if err := am.RecoverInterrupted(); err != nil {
			logger.Fatalf("access: %v", err)
		}
		site.Access = am
	}

	if cfg.Created {
		s := cfg.Get()
		logger.Printf("first run: download client login for Sonarr/Radarr/Lidarr -> username %q, password %q (shown once; rotate it in the admin page)", s.ClientUser, string(s.ClientPassword))
	}
	if k := au.SetupKey(); k != "" {
		logger.Printf("no admin account yet. Open /admin and use setup key: %s", k)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go eng.Run(ctx, env.TickInterval)
	pt := progress.New(cfg, st, logger)
	site.Progress = pt
	site.Storage = storage.New(config.EnvValue("UNDERMYND_STORAGE_ROOT"))
	if root := config.EnvValue("UNDERMYND_STORAGE_ROOT"); root != "" {
		// The folders a re-encode may read and replace files in: the video libraries only.
		for _, f := range []string{"Movies", "Episodes", "Anime"} {
			eng.LibraryRoots = append(eng.LibraryRoots, filepath.Join(root, f))
		}
	}
	site.Storage.Persist(filepath.Join(env.ConfigDir, "storage-scan.json"))
	go site.Storage.Run(ctx, func() time.Duration { return time.Duration(cfg.Get().StorageScanMinutes) * time.Minute })
	go pt.Run(ctx)
	rp := rss.New(cfg, eng, logger)
	site.RSS = rp
	go rp.Run(ctx)
	go watch.New(cfg, eng, logger).Run(ctx)

	servers := []*http.Server{
		{Addr: env.PublicAddr, Handler: site.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second},
		{Addr: env.MachineAddr, Handler: machine, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second},
	}
	errc := make(chan error, len(servers))
	for _, s := range servers {
		go func(s *http.Server) {
			logger.Printf("listening on %s", s.Addr)
			if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- err
			}
		}(s)
	}
	select {
	case <-ctx.Done():
	case err := <-errc:
		logger.Printf("server error: %v", err)
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, s := range servers {
		_ = s.Shutdown(shutdown)
	}
}
