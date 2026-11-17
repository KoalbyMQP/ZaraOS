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
	"cortex/internal/iceoryx"
	"cortex/internal/logger"
	"cortex/internal/middleware"
	"cortex/internal/registry"
	"cortex/internal/requirements"
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


	log.Info("container CLI detected", "cli", handlers.ContainerCLI())

	// Requirements bootloader.
	reqPath := os.Getenv("REQUIREMENTS_PATH")
	bootloader := requirements.NewBootloader(st, reg, log, reqPath)

	// iceoryx RouDi supervisor — started on demand for packages with
	// shared_memory enabled (CycloneDDS SHM), stopped when none run.
	roudi := newRoudiSupervisor(st, log)
	roudi.Cleanup()
	bootloader.SetSharedMemoryProvider(roudi)

	// Public mux — no auth required.
	pub := http.NewServeMux()

	authHandler := handlers.NewAuthHandler(authStore, st, log)
	authHandler.RegisterPublic(pub)
	pub.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Protected mux — all routes require a valid X-Signature.
	prot := http.NewServeMux()

	authHandler.RegisterProtected(prot)
	handlers.NewAppsHandler(reg, log).Register(prot)
	handlers.NewImagesHandler(st, log).Register(prot)
	instancesHandler := handlers.NewInstancesHandler(st, reg, log)
	instancesHandler.SetBootloader(bootloader)
	instancesHandler.Register(prot)
	handlers.NewDiagnosticsHandler(st, log).Register(prot)
	handlers.NewROSHandler(log).Register(prot)
	handlers.NewEventsHandler(st, log).Register(prot)
	handlers.NewIdentityHandler(st, log).Register(prot)
	handlers.NewRequirementsHandler(bootloader, st, reg, log).Register(prot)
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

	// Start background instance reconciler — periodically syncs stored
	// instance state with the actual container runtime so we never report
	// a stale "running" for a container that has already exited.
	reconcileCtx, reconcileCancel := context.WithCancel(context.Background())
	go instancesHandler.StartReconciler(reconcileCtx)
	go roudi.Run(reconcileCtx)

	// Start the requirements control loop in the background — loads the
	// manifest, starts essential packages in dependency order, then
	// continuously reconciles the desired state against reality.
	bootCtx, bootCancel := context.WithCancel(context.Background())
	go func() {
		if err := bootloader.Run(bootCtx); err != nil {
			log.Error("requirements control loop failed", "err", err)
		}
	}()

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	reconcileCancel()
	bootCancel()
	log.Info("shutting down — draining connections...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Error("shutdown error", "err", err)
	}
	log.Info("server stopped")
}

// newRoudiSupervisor builds the iceoryx RouDi supervisor. Users are tracked
// by instance ID against the store. If RouDi dies under running apps, those
// instances are marked crashed and their containers stopped — iceoryx
// clients can't reconnect — so health-checked packages are restarted by the
// bootloader (which starts a fresh RouDi via Acquire).
func newRoudiSupervisor(st *store.Store, log *logger.Logger) *iceoryx.Supervisor {
	state := func(id string) (string, bool) {
		inst, ok := st.GetInstance(id)
		if !ok {
			return "", false
		}
		return inst.State, true
	}
	onCrash := func(ids []string, err error) {
		for _, id := range ids {
			inst, ok := st.GetInstance(id)
			if !ok {
				continue
			}
			exitCode := 1
			st.SetInstanceState(id, "crashed", &exitCode)
			st.SetInstanceError(id, err.Error())
			exec.Command(handlers.ContainerCLI(), "stop", inst.ContainerID).Run()
			st.AddEvent("roudi_crashed", map[string]any{
				"instance_id": id,
				"app":         inst.App,
				"error":       err.Error(),
			})
		}
	}
	return iceoryx.New(iceoryx.Config{SocketDir: requirements.IceoryxSocketDir}, log, state, onCrash)
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
