package controller

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/homarr"
	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/source"
	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/state"
)

type SourceLister interface {
	List(ctx context.Context) ([]source.DashboardEntry, error)
}

type HomarrClient interface {
	ListApps(ctx context.Context) ([]homarr.App, error)
	CreateApp(ctx context.Context, app homarr.AppCreate) (homarr.App, error)
	UpdateApp(ctx context.Context, id string, app homarr.AppUpdate) error
	DeleteApp(ctx context.Context, id string) error
	CreateBoard(ctx context.Context, b homarr.BoardCreate) (homarr.Board, error)
	GetBoardByName(ctx context.Context, name string) (homarr.Board, error)
	SaveBoard(ctx context.Context, b homarr.BoardSave) error
	CreateIntegration(ctx context.Context, i homarr.IntegrationCreate) (homarr.Integration, error)
	DeleteIntegration(ctx context.Context, id string) error
	ListIntegrations(ctx context.Context) ([]homarr.Integration, error)
}

// SecretReader reads Kubernetes Secret data by namespace and name.
type SecretReader interface {
	ReadSecret(ctx context.Context, namespace, name string) (map[string][]byte, error)
}

type ReconcileResult struct {
	Created             int
	Updated             int
	Deleted             int
	IntegrationsCreated int
	IntegrationsDeleted int
}

type Reconciler struct {
	client       HomarrClient
	sources      []SourceLister
	state        *state.InMemoryState
	secretReader SecretReader
	boardName    string
	boardColumns int
	iconBaseURL  string
}

func NewReconciler(client HomarrClient, sources []SourceLister, boardName string, boardColumns int, iconBaseURL string) *Reconciler {
	return &Reconciler{
		client:       client,
		sources:      sources,
		state:        state.NewInMemoryState(),
		boardName:    boardName,
		boardColumns: boardColumns,
		iconBaseURL:  iconBaseURL,
	}
}

func (r *Reconciler) SetSecretReader(sr SecretReader) { r.secretReader = sr }

func (r *Reconciler) State() *state.InMemoryState { return r.state }

func (r *Reconciler) Reconcile(ctx context.Context) (ReconcileResult, error) {
	result := ReconcileResult{}

	// Ensure board exists and get its current state
	board, err := r.client.GetBoardByName(ctx, r.boardName)
	if err != nil {
		if homarr.IsNotFound(err) {
			slog.Info("creating board", "name", r.boardName)
			if _, err = r.client.CreateBoard(ctx, homarr.BoardCreate{
				Name:        r.boardName,
				ColumnCount: r.boardColumns,
				IsPublic:    false,
			}); err != nil {
				return result, fmt.Errorf("create board: %w", err)
			}
			// Re-fetch to get sections and layouts
			board, err = r.client.GetBoardByName(ctx, r.boardName)
			if err != nil {
				return result, fmt.Errorf("get board after create: %w", err)
			}
		} else {
			return result, fmt.Errorf("get board: %w", err)
		}
	}

	// Collect desired entries from all sources
	desired := make(map[string]source.DashboardEntry)
	for _, src := range r.sources {
		entries, err := src.List(ctx)
		if err != nil {
			return result, fmt.Errorf("list source entries: %w", err)
		}
		for _, e := range entries {
			desired[e.ID] = e
		}
	}

	// --- App reconciliation ---

	// Create apps (in desired but not in state)
	for sourceID, entry := range desired {
		if _, exists := r.state.FindAppBySource(sourceID); exists {
			continue
		}
		iconURL := entry.IconURL
		if iconURL != "" && !isFullURL(iconURL) {
			iconURL = r.iconBaseURL + "/" + iconURL + ".svg"
		}
		app, err := r.client.CreateApp(ctx, homarr.AppCreate{
			Name:        entry.Name,
			IconURL:     iconURL,
			Href:        entry.URL,
			Description: entry.Description,
			PingURL:     entry.PingURL,
		})
		if err != nil {
			slog.Error("failed to create app", "source", sourceID, "error", err)
			continue
		}
		r.state.SetApp(app.ID, sourceID)
		result.Created++
	}

	// Delete apps (in state but not in desired)
	for _, homarrID := range r.state.ManagedAppIDs() {
		sourceID := r.state.GetAppSource(homarrID)
		if _, stillDesired := desired[sourceID]; stillDesired {
			continue
		}
		if err := r.client.DeleteApp(ctx, homarrID); err != nil {
			slog.Error("failed to delete app", "homarrID", homarrID, "error", err)
			continue
		}
		r.state.RemoveApp(homarrID)
		result.Deleted++
	}

	// --- Integration reconciliation ---

	// Build desired integrations (entries that have IntegrationType set)
	desiredIntegrations := make(map[string]source.DashboardEntry)
	for sourceID, entry := range desired {
		if entry.IntegrationType != "" {
			desiredIntegrations[sourceID] = entry
		}
	}

	// Create integrations (in desired but not in state)
	for sourceID, entry := range desiredIntegrations {
		if _, exists := r.state.FindIntegrationBySource(sourceID); exists {
			continue
		}

		secrets, err := r.readIntegrationSecrets(ctx, entry)
		if err != nil {
			slog.Error("failed to read integration secret", "source", sourceID, "error", err)
			continue
		}

		intgURL := entry.IntegrationURL
		if intgURL == "" {
			intgURL = entry.URL
		}

		intg, err := r.client.CreateIntegration(ctx, homarr.IntegrationCreate{
			Name:    entry.Name,
			URL:     intgURL,
			Kind:    entry.IntegrationType,
			Secrets: secrets,
		})
		if err != nil {
			slog.Error("failed to create integration", "source", sourceID, "kind", entry.IntegrationType, "error", err)
			continue
		}
		r.state.SetIntegration(intg.ID, sourceID)
		result.IntegrationsCreated++
		slog.Info("created integration", "source", sourceID, "kind", entry.IntegrationType, "id", intg.ID)
	}

	// Delete integrations (in state but not in desired)
	for _, homarrID := range r.state.ManagedIntegrationIDs() {
		sourceID := r.state.GetIntegrationSource(homarrID)
		if _, stillDesired := desiredIntegrations[sourceID]; stillDesired {
			continue
		}
		if err := r.client.DeleteIntegration(ctx, homarrID); err != nil {
			slog.Error("failed to delete integration", "homarrID", homarrID, "error", err)
			continue
		}
		r.state.RemoveIntegration(homarrID)
		result.IntegrationsDeleted++
	}

	// Place apps on the board (with integration linkage)
	if result.Created > 0 || result.Deleted > 0 || result.IntegrationsCreated > 0 || result.IntegrationsDeleted > 0 {
		if err := r.placeAppsOnBoard(ctx, board, desired); err != nil {
			slog.Error("failed to place apps on board", "error", err)
		}
	}

	return result, nil
}

// readIntegrationSecrets reads K8s Secret data for an integration entry.
// If integration-secret-key is set, reads that single key as "apiKey".
// Otherwise, reads all keys from the Secret (key names map to Homarr secret kinds).
func (r *Reconciler) readIntegrationSecrets(ctx context.Context, entry source.DashboardEntry) ([]homarr.IntegrationSecret, error) {
	if r.secretReader == nil {
		return nil, fmt.Errorf("no secret reader configured")
	}
	if entry.IntegrationSecret == "" {
		return nil, nil
	}

	// Extract namespace from the entry ID (format: "namespace/kind/name")
	ns := namespaceFromID(entry.ID)
	if ns == "" {
		return nil, fmt.Errorf("cannot determine namespace from entry ID %q", entry.ID)
	}

	data, err := r.secretReader.ReadSecret(ctx, ns, entry.IntegrationSecret)
	if err != nil {
		return nil, fmt.Errorf("read secret %s/%s: %w", ns, entry.IntegrationSecret, err)
	}

	if entry.IntegrationSecretKey != "" {
		// Single key mode: read the specified key as "apiKey"
		val, ok := data[entry.IntegrationSecretKey]
		if !ok {
			return nil, fmt.Errorf("key %q not found in secret %s/%s", entry.IntegrationSecretKey, ns, entry.IntegrationSecret)
		}
		return []homarr.IntegrationSecret{
			{Kind: "apiKey", Value: string(val)},
		}, nil
	}

	// Multi-key mode: each key in the Secret becomes a Homarr secret kind
	var secrets []homarr.IntegrationSecret
	for k, v := range data {
		secrets = append(secrets, homarr.IntegrationSecret{
			Kind:  k,
			Value: string(v),
		})
	}
	return secrets, nil
}

// placeAppsOnBoard ensures all managed apps are placed as items on the board,
// with integration IDs linked where available.
func (r *Reconciler) placeAppsOnBoard(ctx context.Context, board homarr.Board, desired map[string]source.DashboardEntry) error {
	if len(board.Layouts) == 0 || len(board.Sections) == 0 {
		return fmt.Errorf("board %q has no layouts or sections", board.Name)
	}

	layoutID := board.Layouts[0].ID
	sectionID := board.Sections[0].ID

	// Keep existing items and add new ones for managed apps not yet on board
	items := make([]homarr.BoardItem, len(board.Items))
	copy(items, board.Items)

	col := 0
	row := len(board.Items) // start placing after existing items
	colCount := r.boardColumns
	if colCount <= 0 {
		colCount = 12
	}

	for _, appID := range r.state.ManagedAppIDs() {
		// Check if this app already has an item on the board
		alreadyOnBoard := false
		for _, item := range board.Items {
			if item.Kind == "app" && appIDFromItem(item) == appID {
				alreadyOnBoard = true
				break
			}
		}
		if alreadyOnBoard {
			continue
		}

		// Find integration IDs for this app
		sourceID := r.state.GetAppSource(appID)
		integrationIDs := r.integrationIDsForSource(sourceID)

		// Create a new board item for this app
		itemID := "managed-" + appID
		items = append(items, homarr.BoardItem{
			ID:      itemID,
			Kind:    "app",
			XOffset: col,
			YOffset: row,
			Width:   1,
			Height:  1,
			Options: marshalJSON(map[string]string{
				"appId": appID,
			}),
			AdvancedOptions: marshalJSON(map[string]any{
				"customCssClasses": []string{},
			}),
			IntegrationIDs: integrationIDs,
			Layouts: []homarr.ItemLayout{
				{
					LayoutID:  layoutID,
					SectionID: sectionID,
					XOffset:   col,
					YOffset:   row,
					Width:     1,
					Height:    1,
				},
			},
		})

		col++
		if col >= colCount {
			col = 0
			row++
		}
	}

	// Build sections for save (preserving existing sections)
	sections := make([]homarr.Section, len(board.Sections))
	copy(sections, board.Sections)

	return r.client.SaveBoard(ctx, homarr.BoardSave{
		ID:       board.ID,
		Sections: sections,
		Items:    items,
	})
}

// integrationIDsForSource returns the Homarr integration IDs associated with a source entry.
func (r *Reconciler) integrationIDsForSource(sourceID string) []string {
	intgID, exists := r.state.FindIntegrationBySource(sourceID)
	if !exists {
		return []string{}
	}
	return []string{intgID}
}

func namespaceFromID(id string) string {
	// ID format: "namespace/kind/name"
	for i, c := range id {
		if c == '/' {
			return id[:i]
		}
	}
	return ""
}

func appIDFromItem(item homarr.BoardItem) string {
	if item.Options == nil {
		return ""
	}
	var opts map[string]string
	if err := unmarshalJSON(item.Options, &opts); err != nil {
		return ""
	}
	return opts["appId"]
}

func isFullURL(s string) bool {
	return len(s) >= 8 && (s[:7] == "http://" || s[:8] == "https://")
}
