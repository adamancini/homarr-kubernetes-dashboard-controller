package controller_test

import (
	"context"
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/controller"
	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/homarr"
	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/source"
)

// mockSource returns fixed entries
type mockSource struct {
	entries []source.DashboardEntry
}

func (m *mockSource) List(ctx context.Context) ([]source.DashboardEntry, error) {
	return m.entries, nil
}

// mockHomarrClient records API calls
type mockHomarrClient struct {
	apps         []homarr.App
	integrations []homarr.Integration
	createdApps  []homarr.AppCreate
	deletedApps  []string
	boardExists  bool
}

func (m *mockHomarrClient) ListApps(ctx context.Context) ([]homarr.App, error) {
	return m.apps, nil
}
func (m *mockHomarrClient) CreateApp(ctx context.Context, app homarr.AppCreate) (homarr.App, error) {
	m.createdApps = append(m.createdApps, app)
	return homarr.App{ID: "new-" + app.Name, Name: app.Name, Href: app.Href, IconURL: app.IconURL}, nil
}
func (m *mockHomarrClient) UpdateApp(ctx context.Context, id string, app homarr.AppUpdate) error {
	return nil
}
func (m *mockHomarrClient) DeleteApp(ctx context.Context, id string) error {
	m.deletedApps = append(m.deletedApps, id)
	return nil
}
func (m *mockHomarrClient) CreateBoard(ctx context.Context, b homarr.BoardCreate) (homarr.Board, error) {
	m.boardExists = true
	return homarr.Board{ID: "board-1", Name: b.Name}, nil
}
func (m *mockHomarrClient) GetBoardByName(ctx context.Context, name string) (homarr.Board, error) {
	if m.boardExists {
		return homarr.Board{ID: "board-1", Name: name}, nil
	}
	return homarr.Board{}, &homarr.NotFoundError{Procedure: "board.getBoardByName"}
}
func (m *mockHomarrClient) SaveBoard(ctx context.Context, b homarr.BoardSave) error { return nil }
func (m *mockHomarrClient) CreateIntegration(ctx context.Context, i homarr.IntegrationCreate) (homarr.Integration, error) {
	return homarr.Integration{ID: "intg-" + i.Name, Name: i.Name}, nil
}
func (m *mockHomarrClient) DeleteIntegration(ctx context.Context, id string) error { return nil }
func (m *mockHomarrClient) ListIntegrations(ctx context.Context) ([]homarr.Integration, error) {
	return m.integrations, nil
}

func TestReconciler_CreatesNewApps(t *testing.T) {
	mock := &mockHomarrClient{boardExists: true}
	src := &mockSource{
		entries: []source.DashboardEntry{
			{ID: "htpc/Ingress/sonarr", Name: "Sonarr", URL: "https://sonarr.example.com", IconURL: "sonarr", Group: "Media"},
		},
	}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "default", 12, "https://cdn.example.com/icons/svg")
	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.Created != 1 {
		t.Errorf("Created = %d, want 1", result.Created)
	}
	if len(mock.createdApps) != 1 {
		t.Fatalf("expected 1 created app")
	}
	if mock.createdApps[0].Name != "Sonarr" {
		t.Errorf("created app name = %q", mock.createdApps[0].Name)
	}
}

func TestReconciler_DeletesRemovedApps(t *testing.T) {
	mock := &mockHomarrClient{boardExists: true}
	// No source entries but state has a managed app
	r := controller.NewReconciler(mock, nil, "default", 12, "https://cdn.example.com/icons/svg")
	r.State().SetApp("old-app-id", "htpc/Ingress/removed")

	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", result.Deleted)
	}
	if len(mock.deletedApps) != 1 || mock.deletedApps[0] != "old-app-id" {
		t.Errorf("deletedApps = %v", mock.deletedApps)
	}
}

func TestReconciler_CreatesBoardIfMissing(t *testing.T) {
	mock := &mockHomarrClient{boardExists: false}
	r := controller.NewReconciler(mock, nil, "homelab", 12, "https://cdn.example.com/icons/svg")

	_, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !mock.boardExists {
		t.Error("expected board to be created")
	}
}
