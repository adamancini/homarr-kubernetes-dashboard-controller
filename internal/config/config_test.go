package config_test

import (
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.New()
	if cfg.HomarrURL != "http://homarr.homarr.svc:7575" {
		t.Errorf("unexpected default HomarrURL: %s", cfg.HomarrURL)
	}
	if cfg.BoardName != "default" {
		t.Errorf("unexpected default BoardName: %s", cfg.BoardName)
	}
	if cfg.BoardColumnCount != 12 {
		t.Errorf("unexpected default BoardColumnCount: %d", cfg.BoardColumnCount)
	}
	if cfg.AnnotationPrefix != "homarr.dev" {
		t.Errorf("unexpected default AnnotationPrefix: %s", cfg.AnnotationPrefix)
	}
	if cfg.ReconcileInterval.String() != "5m0s" {
		t.Errorf("unexpected default ReconcileInterval: %s", cfg.ReconcileInterval)
	}
	if !cfg.LeaderElect {
		t.Error("expected LeaderElect to default true")
	}
	if cfg.AllNamespaces {
		t.Error("expected AllNamespaces to default false")
	}
}

func TestConfigValidation(t *testing.T) {
	cfg := config.New()
	// No sources = invalid
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error with no sources")
	}
	cfg.Sources = []string{"ingress"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestConfigValidationBadSource(t *testing.T) {
	cfg := config.New()
	cfg.Sources = []string{"bogus"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for unknown source")
	}
}
