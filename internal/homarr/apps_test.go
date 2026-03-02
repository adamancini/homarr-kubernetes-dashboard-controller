package homarr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/homarr"
)

func TestListApps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/trpc/app.getAll" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("ApiKey") != "test-key" {
			t.Errorf("missing ApiKey header")
		}
		resp := map[string]any{
			"result": map[string]any{
				"data": map[string]any{
					"json": []any{
						map[string]any{"id": "abc", "name": "Sonarr", "iconUrl": "sonarr.svg", "href": "https://sonarr.example.com"},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")
	apps, err := c.ListApps(context.Background())
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}
	if len(apps) != 1 || apps[0].Name != "Sonarr" {
		t.Errorf("unexpected apps: %+v", apps)
	}
}

func TestCreateApp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/trpc/app.create" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			JSON homarr.AppCreate `json:"json"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.JSON.Name != "Sonarr" {
			t.Errorf("unexpected name: %s", body.JSON.Name)
		}
		resp := map[string]any{
			"result": map[string]any{
				"data": map[string]any{
					"json": map[string]any{"id": "new-id", "name": body.JSON.Name, "iconUrl": body.JSON.IconURL, "href": body.JSON.Href},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")
	app, err := c.CreateApp(context.Background(), homarr.AppCreate{
		Name:    "Sonarr",
		IconURL: "sonarr.svg",
		Href:    "https://sonarr.example.com",
	})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	if app.ID != "new-id" {
		t.Errorf("unexpected ID: %s", app.ID)
	}
}

func TestDeleteApp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/trpc/app.delete" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		// tRPC mutation with no result
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{
				"data": map[string]any{
					"json": nil,
				},
			},
		})
	}))
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")
	if err := c.DeleteApp(context.Background(), "abc"); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
}
