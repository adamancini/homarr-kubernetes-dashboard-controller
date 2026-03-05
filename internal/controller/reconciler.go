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

type ReconcileResult struct {
	Created int
	Updated int
	Deleted int
}

type Reconciler struct {
	client       HomarrClient
	sources      []SourceLister
	state        *state.InMemoryState
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

	// Determine what to create (in desired but not in state)
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

	// Determine what to delete (in state but not in desired)
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

	// Place apps on the board if any were created or deleted
	if result.Created > 0 || result.Deleted > 0 {
		if err := r.placeAppsOnBoard(ctx, board); err != nil {
			slog.Error("failed to place apps on board", "error", err)
		}
	}

	return result, nil
}

// placeAppsOnBoard ensures all managed apps are placed as items on the board.
func (r *Reconciler) placeAppsOnBoard(ctx context.Context, board homarr.Board) error {
	if len(board.Layouts) == 0 || len(board.Sections) == 0 {
		return fmt.Errorf("board %q has no layouts or sections", board.Name)
	}

	layoutID := board.Layouts[0].ID
	sectionID := board.Sections[0].ID

	// Build a set of app IDs already on the board
	existingItems := make(map[string]bool)
	for _, item := range board.Items {
		if item.Kind == "app" {
			existingItems[item.ID] = true
		}
	}

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
			IntegrationIDs: []string{},
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
