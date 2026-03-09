package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/config"
	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/controller"
	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/homarr"
	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/source"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	scheme  = runtime.NewScheme()
	version = "dev"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	cfg := config.New()

	var sources, namespaces, ignoreNS stringSlice

	flag.Var(&sources, "source", "Source type to watch (repeatable: ingress, traefik-proxy, gateway-httproute, service)")
	flag.StringVar(&cfg.HomarrURL, "homarr-url", cfg.HomarrURL, "Homarr API base URL")
	flag.StringVar(&cfg.HomarrAPIKeySecret, "homarr-api-key-secret", cfg.HomarrAPIKeySecret, "Secret name containing Homarr API key")
	flag.StringVar(&cfg.HomarrAPIKeyField, "homarr-api-key-secret-key", cfg.HomarrAPIKeyField, "Key within the API key Secret")
	flag.StringVar(&cfg.BoardName, "board-name", cfg.BoardName, "Board to manage")
	flag.IntVar(&cfg.BoardColumnCount, "board-column-count", cfg.BoardColumnCount, "Column count for auto-created board")
	flag.StringVar(&cfg.AnnotationPrefix, "annotation-prefix", cfg.AnnotationPrefix, "Annotation prefix")
	flag.StringVar(&cfg.DefaultIconBaseURL, "default-icon-base-url", cfg.DefaultIconBaseURL, "Base URL for icon name resolution")
	flag.Var(&namespaces, "namespace", "Namespace to watch (repeatable)")
	flag.Var(&ignoreNS, "ignore-namespace", "Namespace to ignore with --all-namespaces (repeatable)")
	flag.BoolVar(&cfg.AllNamespaces, "all-namespaces", cfg.AllNamespaces, "Watch all namespaces")
	flag.DurationVar(&cfg.ReconcileInterval, "reconcile-interval", cfg.ReconcileInterval, "Full resync interval")
	flag.BoolVar(&cfg.LeaderElect, "leader-elect", cfg.LeaderElect, "Enable leader election")
	flag.Parse()

	cfg.Sources = sources
	if len(namespaces) > 0 {
		cfg.Namespaces = namespaces
	}
	if len(ignoreNS) > 0 {
		cfg.IgnoreNS = ignoreNS
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	log := ctrl.Log.WithName("main")

	log.Info("starting homarr-kubernetes-dashboard-controller", "version", version, "sources", cfg.Sources, "board", cfg.BoardName)

	// Build manager options
	mgrOpts := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: ":8080",
		},
		LeaderElection:   cfg.LeaderElect,
		LeaderElectionID: "homarr-dashboard-controller.homarr.dev",
	}

	// Restrict cache to specific namespaces if not watching all
	if !cfg.AllNamespaces && len(cfg.Namespaces) > 0 {
		byNamespace := make(map[string]cache.Config)
		for _, ns := range cfg.Namespaces {
			byNamespace[ns] = cache.Config{}
		}
		mgrOpts.Cache = cache.Options{
			DefaultNamespaces: byNamespace,
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		log.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// Read API key from environment (injected from Secret via Deployment env)
	apiKey := os.Getenv("HOMARR_API_KEY")
	if apiKey == "" {
		log.Error(nil, "HOMARR_API_KEY environment variable is required")
		os.Exit(1)
	}

	homarrClient := homarr.NewClient(cfg.HomarrURL, apiKey)

	// Build source adapters
	var srcAdapters []controller.SourceLister
	watchNS := cfg.Namespaces
	for _, s := range cfg.Sources {
		switch s {
		case "ingress":
			srcAdapters = append(srcAdapters, source.NewIngressSource(mgr.GetClient(), cfg.AnnotationPrefix, watchNS))
		case "traefik-proxy":
			srcAdapters = append(srcAdapters, source.NewIngressRouteSource(mgr.GetClient(), cfg.AnnotationPrefix, watchNS))
		case "service":
			srcAdapters = append(srcAdapters, source.NewServiceSource(mgr.GetClient(), cfg.AnnotationPrefix, watchNS))
		case "gateway-httproute":
			srcAdapters = append(srcAdapters, source.NewHTTPRouteSource(mgr.GetClient(), cfg.AnnotationPrefix, watchNS))
		}
	}

	reconciler := controller.NewReconciler(homarrClient, srcAdapters, cfg.BoardName, cfg.BoardColumnCount, cfg.DefaultIconBaseURL)
	reconciler.SetSecretReader(controller.NewKubeSecretReader(mgr.GetClient()))

	// Add reconciler as a Runnable that runs on a timer
	if err := mgr.Add(&timerRunnable{
		reconciler: reconciler,
		interval:   cfg.ReconcileInterval,
		log:        slog.Default(),
	}); err != nil {
		log.Error(err, "unable to add reconciler runnable")
		os.Exit(1)
	}

	log.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "manager exited with error")
		os.Exit(1)
	}
}

// timerRunnable implements manager.Runnable to run the reconciler on an interval.
type timerRunnable struct {
	reconciler *controller.Reconciler
	interval   time.Duration
	log        *slog.Logger
}

func (t *timerRunnable) Start(ctx context.Context) error {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	// Run immediately on start
	t.runOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			t.runOnce(ctx)
		}
	}
}

func (t *timerRunnable) runOnce(ctx context.Context) {
	result, err := t.reconciler.Reconcile(ctx)
	if err != nil {
		t.log.Error("reconcile failed", "error", err)
		return
	}
	if result.Created > 0 || result.Updated > 0 || result.Deleted > 0 || result.IntegrationsCreated > 0 || result.IntegrationsDeleted > 0 {
		t.log.Info("reconcile complete",
			"created", result.Created, "updated", result.Updated, "deleted", result.Deleted,
			"integrations_created", result.IntegrationsCreated, "integrations_deleted", result.IntegrationsDeleted)
	}
}
