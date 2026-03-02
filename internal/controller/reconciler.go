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

	// Ensure board exists
	_, err := r.client.GetBoardByName(ctx, r.boardName)
	if err != nil {
		if homarr.IsNotFound(err) {
			slog.Info("creating board", "name", r.boardName)
			_, err = r.client.CreateBoard(ctx, homarr.BoardCreate{
				Name:        r.boardName,
				ColumnCount: r.boardColumns,
				IsPublic:    false,
			})
			if err != nil {
				return result, fmt.Errorf("create board: %w", err)
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

	return result, nil
}

func isFullURL(s string) bool {
	return len(s) >= 8 && (s[:7] == "http://" || s[:8] == "https://")
}
