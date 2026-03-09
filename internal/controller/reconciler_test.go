package controller_test

import (
	"context"
	"encoding/json"
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
	board               *homarr.Board // custom board for GetBoardByName
	savedBoard          *homarr.BoardSave
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
	if !m.boardExists {
		return homarr.Board{}, &homarr.NotFoundError{Procedure: "board.getBoardByName"}
	}
	if m.board != nil {
		return *m.board, nil
	}
	return homarr.Board{
		ID:   "board-1",
		Name: name,
		Layouts: []homarr.Layout{
			{ID: "layout-1", Name: "Base", ColumnCount: 12},
		},
		Sections: []homarr.Section{
			{ID: "section-default", Kind: "empty", XOffset: 0, YOffset: 0},
		},
	}, nil
}
func (m *mockHomarrClient) SaveBoard(ctx context.Context, b homarr.BoardSave) error {
	m.savedBoard = &b
	return nil
}
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
			{ID: "htpc/Ingress/sonarr", Name: "Sonarr", URL: "https://sonarr.example.com", IconURL: "sonarr", Category: "Services"},
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
	mock := &mockHomarrClient{
		boardExists: true,
		apps:        []homarr.App{{ID: "old-app-id", Name: "Removed", Href: "https://removed.example.com"}},
	}
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
	mock := &mockHomarrClient{
		boardExists:  true,
		integrations: []homarr.Integration{{ID: "old-intg-id", Name: "Removed", Kind: "sonarr", URL: "http://removed.svc:8989"}},
	}
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

func TestReconciler_AdoptsExistingAppsOnRestart(t *testing.T) {
	// Simulate a restart: Homarr already has the app from a previous run,
	// but the controller's in-memory state is empty (fresh start).
	mock := &mockHomarrClient{
		boardExists: true,
		apps: []homarr.App{
			{ID: "existing-1", Name: "Sonarr", Href: "https://sonarr.example.com"},
		},
	}
	src := &mockSource{
		entries: []source.DashboardEntry{
			{ID: "htpc/Ingress/sonarr", Name: "Sonarr", URL: "https://sonarr.example.com"},
		},
	}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "default", 12, "")
	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// Should adopt the existing app, not create a new one
	if result.Created != 0 {
		t.Errorf("Created = %d, want 0 (should adopt existing)", result.Created)
	}
	if len(mock.createdApps) != 0 {
		t.Errorf("createdApps = %v, want empty", mock.createdApps)
	}
}

func TestReconciler_DeletesDuplicateApps(t *testing.T) {
	// Simulate duplicates from multiple restarts: 3 copies of the same app
	mock := &mockHomarrClient{
		boardExists: true,
		apps: []homarr.App{
			{ID: "app-1", Name: "Bazarr", Href: "https://bazarr.example.com"},
			{ID: "app-2", Name: "Bazarr", Href: "https://bazarr.example.com"},
			{ID: "app-3", Name: "Bazarr", Href: "https://bazarr.example.com"},
		},
	}
	src := &mockSource{
		entries: []source.DashboardEntry{
			{ID: "htpc/Ingress/bazarr", Name: "Bazarr", URL: "https://bazarr.example.com"},
		},
	}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "default", 12, "")
	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.Created != 0 {
		t.Errorf("Created = %d, want 0", result.Created)
	}
	// Should adopt 1 and delete the other 2 duplicates
	if result.Deleted != 2 {
		t.Errorf("Deleted = %d, want 2 (duplicates)", result.Deleted)
	}
	if len(mock.deletedApps) != 2 {
		t.Fatalf("deletedApps = %v, want 2 entries", mock.deletedApps)
	}
	// The first app should be kept, the rest deleted
	for _, id := range mock.deletedApps {
		if id == "app-1" {
			t.Error("should not delete the first (adopted) app")
		}
	}
}

func TestReconciler_ClearsStaleStateOnRestart(t *testing.T) {
	// State references an app that no longer exists in Homarr (manually deleted)
	mock := &mockHomarrClient{
		boardExists: true,
		apps:        []homarr.App{}, // app was deleted from Homarr
	}
	src := &mockSource{
		entries: []source.DashboardEntry{
			{ID: "htpc/Ingress/sonarr", Name: "Sonarr", URL: "https://sonarr.example.com"},
		},
	}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "default", 12, "")
	r.State().SetApp("deleted-from-homarr", "htpc/Ingress/sonarr")

	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// Stale state should be cleared and the app re-created
	if result.Created != 1 {
		t.Errorf("Created = %d, want 1 (should re-create after stale state cleared)", result.Created)
	}
}

func TestReconciler_PlacesAppsInCategorySections(t *testing.T) {
	// Board with Infra and Services category sections (header + empty pairs)
	mock := &mockHomarrClient{
		boardExists: true,
		apps: []homarr.App{
			{ID: "app-flux", Name: "Flux", Href: "https://flux.example.com"},
			{ID: "app-sonarr", Name: "Sonarr", Href: "https://sonarr.example.com"},
			{ID: "app-akkoma", Name: "Akkoma", Href: "https://akkoma.example.com"},
		},
		board: &homarr.Board{
			ID:   "board-1",
			Name: "test",
			Layouts: []homarr.Layout{
				{ID: "layout-1", Name: "Base", ColumnCount: 12},
			},
			Sections: []homarr.Section{
				{ID: "sec-default", Kind: "empty", XOffset: 0, YOffset: 0},
				{ID: "cat-infra", Kind: "category", Name: "Infra", XOffset: 0, YOffset: 1},
				{ID: "sec-infra", Kind: "empty", XOffset: 0, YOffset: 2},
				{ID: "cat-services", Kind: "category", Name: "Services", XOffset: 0, YOffset: 3},
				{ID: "sec-services", Kind: "empty", XOffset: 0, YOffset: 4},
			},
		},
	}

	src := &mockSource{
		entries: []source.DashboardEntry{
			{ID: "flux-system/HTTPRoute/flux", Name: "Flux", URL: "https://flux.example.com", Category: "Infra"},
			{ID: "htpc/Ingress/sonarr", Name: "Sonarr", URL: "https://sonarr.example.com", Category: "Services"},
			{ID: "akkoma/Ingress/akkoma", Name: "Akkoma", URL: "https://akkoma.example.com", Category: "Services"},
		},
	}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "test", 12, "")
	_, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if mock.savedBoard == nil {
		t.Fatal("expected board to be saved")
	}

	// Verify items are placed in the correct sections
	sectionApps := make(map[string][]string) // sectionID -> appIDs
	for _, item := range mock.savedBoard.Items {
		if item.Kind != "app" {
			continue
		}
		for _, l := range item.Layouts {
			appID := ""
			var opts map[string]any
			if err := unmarshalJSONHelper(item.Options, &opts); err == nil {
				if id, ok := opts["appId"].(string); ok {
					appID = id
				}
			}
			sectionApps[l.SectionID] = append(sectionApps[l.SectionID], appID)
		}
	}

	// Flux should be in the Infra section (sec-infra)
	infraApps := sectionApps["sec-infra"]
	if !contains(infraApps, "app-flux") {
		t.Errorf("Infra section apps = %v, want app-flux", infraApps)
	}

	// Sonarr and Akkoma should be in the Services section (sec-services)
	servicesApps := sectionApps["sec-services"]
	if !contains(servicesApps, "app-sonarr") {
		t.Errorf("Services section apps = %v, want app-sonarr", servicesApps)
	}
	if !contains(servicesApps, "app-akkoma") {
		t.Errorf("Services section apps = %v, want app-akkoma", servicesApps)
	}

	// Nothing should be in the default section
	defaultApps := sectionApps["sec-default"]
	if len(defaultApps) != 0 {
		t.Errorf("Default section should be empty, got %v", defaultApps)
	}
}

func TestReconciler_CreatesNewCategorySection(t *testing.T) {
	// Board with no category sections — controller should create them
	mock := &mockHomarrClient{
		boardExists: true,
		board: &homarr.Board{
			ID:   "board-1",
			Name: "test",
			Layouts: []homarr.Layout{
				{ID: "layout-1", Name: "Base", ColumnCount: 12},
			},
			Sections: []homarr.Section{
				{ID: "sec-default", Kind: "empty", XOffset: 0, YOffset: 0},
			},
		},
	}

	src := &mockSource{
		entries: []source.DashboardEntry{
			{ID: "htpc/Ingress/sonarr", Name: "Sonarr", URL: "https://sonarr.example.com", Category: "Services"},
		},
	}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "test", 12, "")
	_, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if mock.savedBoard == nil {
		t.Fatal("expected board to be saved")
	}

	// Should have created a new category section + empty section
	hasCat := false
	for _, s := range mock.savedBoard.Sections {
		if s.Kind == "category" && s.Name == "Services" {
			hasCat = true
			break
		}
	}
	if !hasCat {
		t.Error("expected Services category section to be created")
	}
}

func TestReconciler_MovesAppToCorrectCategory(t *testing.T) {
	// App is already on the board in the wrong section; reconcile should move it
	mock := &mockHomarrClient{
		boardExists: true,
		apps: []homarr.App{
			{ID: "app-flux", Name: "Flux", Href: "https://flux.example.com"},
		},
		board: &homarr.Board{
			ID:   "board-1",
			Name: "test",
			Layouts: []homarr.Layout{
				{ID: "layout-1", Name: "Base", ColumnCount: 12},
			},
			Sections: []homarr.Section{
				{ID: "sec-default", Kind: "empty", XOffset: 0, YOffset: 0},
				{ID: "cat-infra", Kind: "category", Name: "Infra", XOffset: 0, YOffset: 1},
				{ID: "sec-infra", Kind: "empty", XOffset: 0, YOffset: 2},
				{ID: "cat-services", Kind: "category", Name: "Services", XOffset: 0, YOffset: 3},
				{ID: "sec-services", Kind: "empty", XOffset: 0, YOffset: 4},
			},
			Items: []homarr.BoardItem{
				{
					ID:      "managed-app-flux",
					Kind:    "app",
					Options: mustMarshal(map[string]string{"appId": "app-flux"}),
					Layouts: []homarr.ItemLayout{
						{LayoutID: "layout-1", SectionID: "sec-services", XOffset: 0, YOffset: 0, Width: 1, Height: 1},
					},
					IntegrationIDs: []string{},
				},
			},
		},
	}

	src := &mockSource{
		entries: []source.DashboardEntry{
			{ID: "flux-system/HTTPRoute/flux", Name: "Flux", URL: "https://flux.example.com", Category: "Infra"},
		},
	}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "test", 12, "")
	_, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if mock.savedBoard == nil {
		t.Fatal("expected board to be saved")
	}

	// Flux should now be in the Infra section, not Services
	for _, item := range mock.savedBoard.Items {
		if item.Kind != "app" {
			continue
		}
		var opts map[string]any
		if err := unmarshalJSONHelper(item.Options, &opts); err == nil {
			if id, ok := opts["appId"].(string); ok && id == "app-flux" {
				for _, l := range item.Layouts {
					if l.SectionID != "sec-infra" {
						t.Errorf("Flux sectionID = %q, want sec-infra (should be moved)", l.SectionID)
					}
				}
			}
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func unmarshalJSONHelper(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
