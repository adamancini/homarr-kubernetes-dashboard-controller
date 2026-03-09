package source_test

import (
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/source"
)

func TestParseAnnotations(t *testing.T) {
	annotations := map[string]string{
		"homarr.dev/enabled":                "true",
		"homarr.dev/name":                   "Sonarr",
		"homarr.dev/icon":                   "sonarr",
		"homarr.dev/category":               "Services",
		"homarr.dev/description":            "TV show manager",
		"homarr.dev/priority":               "10",
		"homarr.dev/ping-url":               "http://sonarr:8989/ping",
		"homarr.dev/integration-type":       "sonarr",
		"homarr.dev/integration-url":        "http://sonarr:8989",
		"homarr.dev/integration-secret":     "sonarr-api-key",
		"homarr.dev/integration-secret-key": "api-key",
		"homarr.dev/widget":                 "downloads",
		"homarr.dev/url":                    "https://sonarr.example.com",
	}

	entry := source.ParseAnnotations(annotations, "homarr.dev")

	if entry.Name != "Sonarr" {
		t.Errorf("Name = %q, want Sonarr", entry.Name)
	}
	if entry.IconURL != "sonarr" {
		t.Errorf("IconURL = %q, want sonarr", entry.IconURL)
	}
	if entry.Category != "Services" {
		t.Errorf("Category = %q, want Services", entry.Category)
	}
	if entry.Description != "TV show manager" {
		t.Errorf("Description = %q", entry.Description)
	}
	if entry.Priority != 10 {
		t.Errorf("Priority = %d, want 10", entry.Priority)
	}
	if entry.PingURL != "http://sonarr:8989/ping" {
		t.Errorf("PingURL = %q", entry.PingURL)
	}
	if entry.IntegrationType != "sonarr" {
		t.Errorf("IntegrationType = %q", entry.IntegrationType)
	}
	if entry.IntegrationURL != "http://sonarr:8989" {
		t.Errorf("IntegrationURL = %q", entry.IntegrationURL)
	}
	if entry.IntegrationSecret != "sonarr-api-key" {
		t.Errorf("IntegrationSecret = %q", entry.IntegrationSecret)
	}
	if entry.IntegrationSecretKey != "api-key" {
		t.Errorf("IntegrationSecretKey = %q", entry.IntegrationSecretKey)
	}
	if entry.Widget != "downloads" {
		t.Errorf("Widget = %q", entry.Widget)
	}
	if entry.URL != "https://sonarr.example.com" {
		t.Errorf("URL = %q", entry.URL)
	}
}

func TestParseAnnotations_GroupFallback(t *testing.T) {
	// Legacy "group" annotation should still work as fallback
	annotations := map[string]string{
		"homarr.dev/enabled": "true",
		"homarr.dev/name":    "Legacy",
		"homarr.dev/group":   "OldGroup",
	}
	entry := source.ParseAnnotations(annotations, "homarr.dev")
	if entry.Category != "OldGroup" {
		t.Errorf("Category = %q, want OldGroup (from group fallback)", entry.Category)
	}
}

func TestParseAnnotations_CategoryOverridesGroup(t *testing.T) {
	// When both are set, category takes precedence
	annotations := map[string]string{
		"homarr.dev/enabled":  "true",
		"homarr.dev/name":     "Both",
		"homarr.dev/category": "NewCategory",
		"homarr.dev/group":    "OldGroup",
	}
	entry := source.ParseAnnotations(annotations, "homarr.dev")
	if entry.Category != "NewCategory" {
		t.Errorf("Category = %q, want NewCategory (category should override group)", entry.Category)
	}
}

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		annotations map[string]string
		want        bool
	}{
		{map[string]string{"homarr.dev/enabled": "true"}, true},
		{map[string]string{"homarr.dev/enabled": "false"}, false},
		{map[string]string{}, false},
		{nil, false},
	}
	for _, tt := range tests {
		got := source.IsEnabled(tt.annotations, "homarr.dev")
		if got != tt.want {
			t.Errorf("IsEnabled(%v) = %v, want %v", tt.annotations, got, tt.want)
		}
	}
}
