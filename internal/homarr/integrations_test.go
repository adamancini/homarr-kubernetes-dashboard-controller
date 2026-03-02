package homarr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/homarr"
)

func TestCreateIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/trpc/integration.create" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]any{
			"result": map[string]any{
				"data": map[string]any{
					"json": map[string]any{"id": "intg-1", "name": "Sonarr", "url": "http://sonarr:8989", "kind": "sonarr"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")
	intg, err := c.CreateIntegration(context.Background(), homarr.IntegrationCreate{
		Name: "Sonarr",
		URL:  "http://sonarr:8989",
		Kind: "sonarr",
		Secrets: []homarr.IntegrationSecret{
			{Kind: "apiKey", Value: "secret123"},
		},
	})
	if err != nil {
		t.Fatalf("CreateIntegration: %v", err)
	}
	if intg.ID != "intg-1" {
		t.Errorf("unexpected ID: %s", intg.ID)
	}
}

func TestListIntegrations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"result": map[string]any{
				"data": map[string]any{
					"json": []any{
						map[string]any{"id": "intg-1", "name": "Sonarr", "url": "http://sonarr:8989", "kind": "sonarr"},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")
	intgs, err := c.ListIntegrations(context.Background())
	if err != nil {
		t.Fatalf("ListIntegrations: %v", err)
	}
	if len(intgs) != 1 {
		t.Errorf("expected 1 integration, got %d", len(intgs))
	}
}
