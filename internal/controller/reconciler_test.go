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

// mockSecretReader returns preset secret data
type mockSecretReader struct {
	secrets map[string]map[string][]byte // "namespace/name" -> data
}

func (m *mockSecretReader) ReadSecret(ctx context.Context, namespace, name string) (map[string][]byte, error) {
	key := namespace + "/" + name
	if data, ok := m.secrets[key]; ok {
		return data, nil
	}
	return nil, nil
}

// mockHomarrClient records API calls
type mockHomarrClient struct {
	apps                []homarr.App
	integrations        []homarr.Integration
	createdApps         []homarr.AppCreate
	deletedApps         []string
	createdIntegrations []homarr.IntegrationCreate
	deletedIntegrations []string
	boardExists         bool
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
	m.createdIntegrations = append(m.createdIntegrations, i)
	return homarr.Integration{ID: "intg-" + i.Name, Name: i.Name, Kind: i.Kind, URL: i.URL}, nil
}
func (m *mockHomarrClient) DeleteIntegration(ctx context.Context, id string) error {
	m.deletedIntegrations = append(m.deletedIntegrations, id)
	return nil
}
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

func TestReconciler_CreatesIntegration(t *testing.T) {
	mock := &mockHomarrClient{boardExists: true}
	secrets := &mockSecretReader{
		secrets: map[string]map[string][]byte{
			"htpc/sonarr-api-key": {"api-key": []byte("test-key-123")},
		},
	}
	src := &mockSource{
		entries: []source.DashboardEntry{
			{
				ID:                   "htpc/Ingress/sonarr",
				Name:                 "Sonarr",
				URL:                  "https://sonarr.example.com",
				IntegrationType:      "sonarr",
				IntegrationURL:       "http://sonarr.htpc.svc:8989",
				IntegrationSecret:    "sonarr-api-key",
				IntegrationSecretKey: "api-key",
			},
		},
	}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "default", 12, "")
	r.SetSecretReader(secrets)
	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.Created != 1 {
		t.Errorf("Created = %d, want 1", result.Created)
	}
	if result.IntegrationsCreated != 1 {
		t.Errorf("IntegrationsCreated = %d, want 1", result.IntegrationsCreated)
	}
	if len(mock.createdIntegrations) != 1 {
		t.Fatalf("expected 1 created integration, got %d", len(mock.createdIntegrations))
	}
	intg := mock.createdIntegrations[0]
	if intg.Kind != "sonarr" {
		t.Errorf("integration kind = %q, want sonarr", intg.Kind)
	}
	if intg.URL != "http://sonarr.htpc.svc:8989" {
		t.Errorf("integration URL = %q", intg.URL)
	}
	if len(intg.Secrets) != 1 || intg.Secrets[0].Kind != "apiKey" || intg.Secrets[0].Value != "test-key-123" {
		t.Errorf("integration secrets = %+v", intg.Secrets)
	}
}

func TestReconciler_DeletesRemovedIntegrations(t *testing.T) {
	mock := &mockHomarrClient{boardExists: true}
	r := controller.NewReconciler(mock, nil, "default", 12, "")
	r.State().SetIntegration("old-intg-id", "htpc/Ingress/removed")

	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.IntegrationsDeleted != 1 {
		t.Errorf("IntegrationsDeleted = %d, want 1", result.IntegrationsDeleted)
	}
	if len(mock.deletedIntegrations) != 1 || mock.deletedIntegrations[0] != "old-intg-id" {
		t.Errorf("deletedIntegrations = %v", mock.deletedIntegrations)
	}
}

func TestReconciler_MultiKeySecrets(t *testing.T) {
	mock := &mockHomarrClient{boardExists: true}
	secrets := &mockSecretReader{
		secrets: map[string]map[string][]byte{
			"htpc/qbit-creds": {
				"username": []byte("admin"),
				"password": []byte("secret"),
			},
		},
	}
	src := &mockSource{
		entries: []source.DashboardEntry{
			{
				ID:                "htpc/Ingress/qbittorrent",
				Name:              "qBittorrent",
				URL:               "https://qbit.example.com",
				IntegrationType:   "qBittorrent",
				IntegrationURL:    "http://qbittorrent.htpc.svc:8080",
				IntegrationSecret: "qbit-creds",
			},
		},
	}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "default", 12, "")
	r.SetSecretReader(secrets)
	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.IntegrationsCreated != 1 {
		t.Errorf("IntegrationsCreated = %d, want 1", result.IntegrationsCreated)
	}
	if len(mock.createdIntegrations) != 1 {
		t.Fatalf("expected 1 integration")
	}
	intg := mock.createdIntegrations[0]
	if len(intg.Secrets) != 2 {
		t.Fatalf("expected 2 secrets, got %d", len(intg.Secrets))
	}
	kinds := make(map[string]string)
	for _, s := range intg.Secrets {
		kinds[s.Kind] = s.Value
	}
	if kinds["username"] != "admin" {
		t.Errorf("username = %q, want admin", kinds["username"])
	}
	if kinds["password"] != "secret" {
		t.Errorf("password = %q, want secret", kinds["password"])
	}
}

func TestReconciler_IntegrationFallsBackToEntryURL(t *testing.T) {
	mock := &mockHomarrClient{boardExists: true}
	secrets := &mockSecretReader{
		secrets: map[string]map[string][]byte{
			"htpc/plex-token": {"token": []byte("plex-123")},
		},
	}
	src := &mockSource{
		entries: []source.DashboardEntry{
			{
				ID:                   "htpc/Ingress/plex",
				Name:                 "Plex",
				URL:                  "https://plex.example.com",
				IntegrationType:      "plex",
				IntegrationSecret:    "plex-token",
				IntegrationSecretKey: "token",
			},
		},
	}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "default", 12, "")
	r.SetSecretReader(secrets)
	_, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(mock.createdIntegrations) != 1 {
		t.Fatalf("expected 1 integration")
	}
	if mock.createdIntegrations[0].URL != "https://plex.example.com" {
		t.Errorf("integration URL = %q, want https://plex.example.com", mock.createdIntegrations[0].URL)
	}
}

func TestReconciler_SkipsIntegrationWithoutSecretReader(t *testing.T) {
	mock := &mockHomarrClient{boardExists: true}
	src := &mockSource{
		entries: []source.DashboardEntry{
			{
				ID:                "htpc/Ingress/sonarr",
				Name:              "Sonarr",
				URL:               "https://sonarr.example.com",
				IntegrationType:   "sonarr",
				IntegrationSecret: "sonarr-api-key",
			},
		},
	}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "default", 12, "")
	// No secret reader set
	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.Created != 1 {
		t.Errorf("Created = %d, want 1", result.Created)
	}
	if result.IntegrationsCreated != 0 {
		t.Errorf("IntegrationsCreated = %d, want 0", result.IntegrationsCreated)
	}
}
