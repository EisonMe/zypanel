package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/EisonMe/zypanel/internal/audit"
	"github.com/EisonMe/zypanel/internal/auth"
	"github.com/EisonMe/zypanel/internal/config"
	"github.com/EisonMe/zypanel/internal/version"
	"github.com/EisonMe/zypanel/internal/web"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	authMgr, err := auth.NewManager(cfg.AdminUser, cfg.AdminPassword, cfg.AdminPasswordHash, cfg.SecretKey, cfg.SessionTTL)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	renderer, err := web.NewRenderer()
	if err != nil {
		log.Fatalf("templates: %v", err)
	}

	srv := &web.Server{
		Renderer:                   renderer,
		Auth:                       authMgr,
		DockerEnabled:              cfg.DockerEnabled,
		SystemdEnabled:             cfg.SystemdEnabled,
		CloudflaredEnabled:         cfg.CloudflaredEnabled,
		OpenClawEnabled:            cfg.OpenClawEnabled,
		ReadOnly:                   cfg.ReadOnly,
		DockerBin:                  cfg.DockerBin,
		SystemctlBin:               cfg.SystemctlBin,
		CloudflaredBin:             cfg.CloudflaredBin,
		CloudflaredConfig:          cfg.CloudflaredConfig,
		OpenClawBin:                cfg.OpenClawBin,
		CloudflaredSuffixAllowlist: cfg.CloudflaredSuffixAllowlist,
		Audit:                      audit.New(cfg.AuditLogPath),
		Version:                    version.Version,
		Addr:                       cfg.Addr,
		DataDir:                    cfg.DataDir,
		AuditLog:                   cfg.AuditLogPath,
		CookieSecure:               cfg.CookieSecure,
		GoVersion:                  runtime.Version(),
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		log.Printf("zypanel %s listening on %s", version.String(), cfg.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	log.Println("stopped")
}
