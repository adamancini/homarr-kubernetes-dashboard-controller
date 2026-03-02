package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/config"
)

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

	_ = cfg
	fmt.Printf("homarr-kubernetes-dashboard-controller starting (sources=%v, board=%s)\n",
		cfg.Sources, cfg.BoardName)
}

var version = "dev"

func init() {
	_ = version
	_ = time.Second // ensure time is used
}
