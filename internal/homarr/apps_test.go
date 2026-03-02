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
		if r.URL.Path != "/api/apps" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("ApiKey") != "test-key" {
			t.Errorf("missing ApiKey header")
		}
		json.NewEncoder(w).Encode([]homarr.App{
			{ID: "abc", Name: "Sonarr", IconURL: "sonarr.svg", Href: "https://sonarr.example.com"},
		})
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
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body homarr.AppCreate
		json.NewDecoder(r.Body).Decode(&body)
		if body.Name != "Sonarr" {
			t.Errorf("unexpected name: %s", body.Name)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(homarr.App{ID: "new-id", Name: body.Name, IconURL: body.IconURL, Href: body.Href})
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
		if r.Method != http.MethodDelete || r.URL.Path != "/api/apps/abc" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")
	if err := c.DeleteApp(context.Background(), "abc"); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
}
