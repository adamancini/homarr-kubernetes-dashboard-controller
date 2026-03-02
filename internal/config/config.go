package config

import (
	"fmt"
	"strings"
	"time"
)

var validSources = map[string]bool{
	"ingress":           true,
	"traefik-proxy":     true,
	"gateway-httproute": true,
	"service":           true,
}

type Config struct {
	// Sources to watch
	Sources []string

	// Homarr connection
	HomarrURL          string
	HomarrAPIKeySecret string
	HomarrAPIKeyField  string

	// Board settings
	BoardName        string
	BoardColumnCount int

	// Annotation prefix
	AnnotationPrefix string

	// Icon resolution
	DefaultIconBaseURL string

	// Namespace scoping
	Namespaces    []string
	IgnoreNS      []string
	AllNamespaces bool

	// Controller behavior
	ReconcileInterval time.Duration
	LeaderElect       bool
}

func New() *Config {
	return &Config{
		HomarrURL:          "http://homarr.homarr.svc:7575",
		HomarrAPIKeySecret: "homarr-api-key",
		HomarrAPIKeyField:  "api-key",
		BoardName:          "default",
		BoardColumnCount:   12,
		AnnotationPrefix:   "homarr.dev",
		DefaultIconBaseURL: "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/svg",
		IgnoreNS:           []string{"kube-system", "flux-system"},
		ReconcileInterval:  5 * time.Minute,
		LeaderElect:        true,
	}
}

func (c *Config) Validate() error {
	if len(c.Sources) == 0 {
		return fmt.Errorf("at least one --source is required")
	}
	for _, s := range c.Sources {
		if !validSources[strings.TrimSpace(s)] {
			return fmt.Errorf("unknown source %q (valid: ingress, traefik-proxy, gateway-httproute, service)", s)
		}
	}
	return nil
}
