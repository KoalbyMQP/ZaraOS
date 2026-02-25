package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"cortex/internal/auth"
	"cortex/internal/handlers"
	"cortex/internal/logger"
	"cortex/internal/middleware"
	"cortex/internal/registry"
	"cortex/internal/store"
)

const addr = ":8080"

func main() {
	log := logger.New()
	st := store.New()
	authStore := auth.NewStore()
	reg := registry.New()

	// Eagerly populate cache on boot so apps are ready immediately.
	log.Info("fetching app registry...")
	apps := reg.ListApps()
	log.Info("registry ready", "apps", len(apps))


	checkNerdctl(log)

	// Public mux — no auth required.
	pub := http.NewServeMux()

	authHandler := handlers.NewAuthHandler(authStore, st, log)
	authHandler.RegisterPublic(pub)

	// Protected mux — all routes require a valid X-Signature.
	prot := http.NewServeMux()

	authHandler.RegisterProtected(prot)
	handlers.NewAppsHandler(reg, log).Register(prot)
	handlers.NewInstancesHandler(st, reg, log).Register(prot)
	handlers.NewDiagnosticsHandler(st, log).Register(prot)
	handlers.NewROSHandler(log).Register(prot)
	handlers.NewEventsHandler(st, log).Register(prot)
	handlers.NewIdentityHandler(st, log).Register(prot)
	handlers.NewShellHandler(log).Register(prot, pub)

	prot.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})

	pub.Handle("/", middleware.RequireAuth(authStore, log, prot))

	server := &http.Server{
		Addr:         addr,
		Handler:      middleware.CORS(middleware.Logging(log, pub)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	printBanner(log)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down — draining connections...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Error("shutdown error", "err", err)
	}
	log.Info("server stopped")
}

func checkNerdctl(log *logger.Logger) {
	if _, err := exec.LookPath("nerdctl"); err != nil {
		log.Warn("nerdctl not found in PATH — instance start/stop will fail",
			"hint", "install nerdctl and ensure containerd is running")
	} else {
		log.Info("nerdctl found")
	}
}

func printBanner(log *logger.Logger) {
	log.Banner(
		"  .g8'''bgd                   mm                     ",
		".dP'     `M                   MM                     ",
		"dM'       ` ,pW'Wq.`7Mb,od8 mmMMmm .gP'Ya `7M'   `MF'",
		"MM         6W'   `Wb MM' ''   MM  ,M'   Yb  `VA ,V'  ",
		"MM.        8M     M8 MM       MM  8M''''''    XMX    ",
		"`Mb.     ,'YA.   ,A9 MM       MM  YM.    ,  ,V' VA.  ",
		"  `'bmmmd'  `Ybmd9'.JMML.     `Mbmo`Mbmmd'.AM.   .MA.",
	)

	log.Info("listening", "addr", addr)
	fmt.Println()
}
