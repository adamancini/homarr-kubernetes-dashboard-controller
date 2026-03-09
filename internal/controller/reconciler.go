package controller

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

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

	// Ensure board exists (created if missing; actual board data is
	// fetched fresh before placement at the end of the reconcile loop).
	if err := r.ensureBoardExists(ctx); err != nil {
		return result, err
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

	// --- Adopt existing Homarr state ---
	// Fetch all apps from Homarr and match them to desired entries. This
	// prevents re-creating apps after controller restarts (when in-memory
	// state is lost) and cleans up any duplicates from previous restarts.
	existingApps, err := r.client.ListApps(ctx)
	if err != nil {
		return result, fmt.Errorf("list existing apps: %w", err)
	}
	result.Deleted += r.adoptAndDedupApps(ctx, existingApps, desired)

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

	// Adopt existing integrations (same rationale as apps above)
	existingIntegrations, err := r.client.ListIntegrations(ctx)
	if err != nil {
		slog.Error("failed to list existing integrations", "error", err)
	} else {
		result.IntegrationsDeleted += r.adoptAndDedupIntegrations(ctx, existingIntegrations, desiredIntegrations)
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

	// Ensure all managed apps are placed on the board.
	board, err := r.client.GetBoardByName(ctx, r.boardName)
	if err != nil {
		slog.Error("failed to re-fetch board for placement", "error", err)
	} else if err := r.placeAppsOnBoard(ctx, board, desired); err != nil {
		slog.Error("failed to place apps on board", "error", err)
	}

	return result, nil
}

// ensureBoardExists checks that the target board exists and creates it if not.
func (r *Reconciler) ensureBoardExists(ctx context.Context) error {
	_, err := r.client.GetBoardByName(ctx, r.boardName)
	if err == nil {
		return nil
	}
	if !homarr.IsNotFound(err) {
		return fmt.Errorf("get board: %w", err)
	}
	slog.Info("creating board", "name", r.boardName)
	if _, err := r.client.CreateBoard(ctx, homarr.BoardCreate{
		Name:        r.boardName,
		ColumnCount: r.boardColumns,
		IsPublic:    false,
	}); err != nil {
		return fmt.Errorf("create board: %w", err)
	}
	return nil
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

// placeAppsOnBoard ensures all managed apps are placed as items on the board
// in the correct category section, with integration IDs linked where available.
// Apps whose category annotation changed are moved to the correct section.
func (r *Reconciler) placeAppsOnBoard(ctx context.Context, board homarr.Board, desired map[string]source.DashboardEntry) error {
	if len(board.Layouts) == 0 || len(board.Sections) == 0 {
		return fmt.Errorf("board %q has no layouts or sections", board.Name)
	}

	layoutID := board.Layouts[0].ID

	colCount := r.boardColumns
	if colCount <= 0 {
		colCount = 12
	}

	sections := make([]homarr.Section, len(board.Sections))
	copy(sections, board.Sections)

	// Build section lookup: category name -> section ID of the empty section
	// that follows the category header. In Homarr, a "category" section is a
	// header; items go in the next "empty" section that follows it.
	categorySectionID := r.buildCategorySectionMap(sections)

	// Build reverse lookup: appID -> sourceID for category resolution
	appCategory := make(map[string]string) // appID -> desired category
	for _, appID := range r.state.ManagedAppIDs() {
		sourceID := r.state.GetAppSource(appID)
		if entry, ok := desired[sourceID]; ok {
			appCategory[appID] = entry.Category
		}
	}

	// Collect categories that don't have sections yet
	newCategories := make(map[string]bool)
	for _, cat := range appCategory {
		if cat == "" {
			continue
		}
		if _, exists := categorySectionID[cat]; !exists {
			newCategories[cat] = true
		}
	}

	// Create missing category sections (header + empty content section)
	if len(newCategories) > 0 {
		maxY := 0
		for _, s := range sections {
			if s.YOffset > maxY {
				maxY = s.YOffset
			}
		}
		for cat := range newCategories {
			maxY++
			catSection := homarr.Section{
				ID:      generateSectionID("cat", cat),
				Kind:    "category",
				Name:    cat,
				XOffset: 0,
				YOffset: maxY,
			}
			maxY++
			emptySection := homarr.Section{
				ID:      generateSectionID("sec", cat),
				Kind:    "empty",
				XOffset: 0,
				YOffset: maxY,
			}
			sections = append(sections, catSection, emptySection)
			categorySectionID[cat] = emptySection.ID
			slog.Info("created category section", "category", cat)
		}
	}

	// Determine the default section (first "empty" section) for uncategorized apps
	defaultSectionID := board.Sections[0].ID

	// Keep only non-managed items and track which non-managed apps are on the board
	onBoard := make(map[string]bool)
	var items []homarr.BoardItem
	for _, item := range board.Items {
		if item.Kind == "app" && strings.HasPrefix(item.ID, "managed-") {
			continue // drop all managed items — we rebuild them below
		}
		items = append(items, item)
		if item.Kind == "app" {
			if id := appIDFromItem(item); id != "" {
				onBoard[id] = true
			}
		}
	}

	baseLen := len(items)

	// Track per-section grid cursors for layout positioning
	type cursor struct{ col, row int }
	cursors := make(map[string]*cursor)
	// Seed cursors from existing non-managed items in each section
	for _, item := range items {
		for _, l := range item.Layouts {
			c, ok := cursors[l.SectionID]
			if !ok {
				c = &cursor{}
				cursors[l.SectionID] = c
			}
			endCol := l.XOffset + l.Width
			if l.YOffset > c.row || (l.YOffset == c.row && endCol > c.col) {
				c.row = l.YOffset
				c.col = endCol
			}
		}
	}

	// Add items for all managed apps, placing each in its category section
	for _, appID := range r.state.ManagedAppIDs() {
		if onBoard[appID] {
			continue
		}

		sourceID := r.state.GetAppSource(appID)
		integrationIDs := r.integrationIDsForSource(sourceID)

		// Resolve target section from category
		targetSection := defaultSectionID
		if cat := appCategory[appID]; cat != "" {
			if secID, ok := categorySectionID[cat]; ok {
				targetSection = secID
			}
		}

		// Get or create cursor for this section
		c, ok := cursors[targetSection]
		if !ok {
			c = &cursor{}
			cursors[targetSection] = c
		}
		if c.col >= colCount {
			c.col = 0
			c.row++
		}

		items = append(items, homarr.BoardItem{
			ID:   "managed-" + appID,
			Kind: "app",
			Options: marshalJSON(map[string]string{
				"appId": appID,
			}),
			AdvancedOptions: marshalJSON(map[string]any{
				"title":            nil,
				"customCssClasses": []string{},
				"borderColor":      "",
			}),
			IntegrationIDs: integrationIDs,
			Layouts: []homarr.ItemLayout{
				{
					LayoutID:  layoutID,
					SectionID: targetSection,
					XOffset:   c.col,
					YOffset:   c.row,
					Width:     1,
					Height:    1,
				},
			},
		})

		c.col++
	}

	managedCount := len(items) - baseLen
	oldManagedCount := len(board.Items) - baseLen
	slog.Info("board item audit",
		"nonManaged", baseLen, "managed", managedCount,
		"previousManaged", oldManagedCount, "total", len(items))

	// Save if the managed items changed (count or content)
	if managedCount == oldManagedCount && managedCount == 0 {
		slog.Info("board placement: no changes needed")
		return nil
	}

	slog.Info("saving board", "items", len(items))

	return r.client.SaveBoard(ctx, homarr.BoardSave{
		ID:       board.ID,
		Sections: sections,
		Items:    items,
	})
}

// buildCategorySectionMap maps category names to the "empty" section ID that
// follows each "category" header section on the board. Items are placed in
// the empty section, not in the category header itself.
func (r *Reconciler) buildCategorySectionMap(sections []homarr.Section) map[string]string {
	result := make(map[string]string)

	// Sort by YOffset to find the empty section after each category
	type indexedSection struct {
		idx int
		sec homarr.Section
	}
	sorted := make([]indexedSection, len(sections))
	for i, s := range sections {
		sorted[i] = indexedSection{i, s}
	}
	// Stable sort by YOffset
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].sec.YOffset < sorted[j-1].sec.YOffset; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	for i, is := range sorted {
		if is.sec.Kind != "category" || is.sec.Name == "" {
			continue
		}
		// Find the next empty section after this category
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].sec.Kind == "empty" {
				result[is.sec.Name] = sorted[j].sec.ID
				break
			}
		}
	}

	return result
}

// generateSectionID creates a deterministic section ID for new categories.
func generateSectionID(prefix, name string) string {
	// Simple hash-based ID to be deterministic across reconciles
	h := uint32(0)
	for _, c := range name {
		h = h*31 + uint32(c)
	}
	return fmt.Sprintf("%s-%s-%08x", prefix, strings.ToLower(strings.ReplaceAll(name, " ", "-")), h)
}

// adoptAndDedupApps matches existing Homarr apps to desired entries by name+href,
// adopts the first match into state, and deletes duplicates. Returns the number
// of duplicates deleted.
func (r *Reconciler) adoptAndDedupApps(ctx context.Context, existing []homarr.App, desired map[string]source.DashboardEntry) int {
	type appKey struct{ name, href string }

	// Group existing apps by name+href
	byKey := make(map[appKey][]homarr.App)
	for _, app := range existing {
		k := appKey{app.Name, app.Href}
		byKey[k] = append(byKey[k], app)
	}

	// Remove stale entries from state (apps deleted from Homarr externally)
	validIDs := make(map[string]bool, len(existing))
	for _, app := range existing {
		validIDs[app.ID] = true
	}
	for _, homarrID := range r.state.ManagedAppIDs() {
		if !validIDs[homarrID] {
			r.state.RemoveApp(homarrID)
		}
	}

	deleted := 0
	for sourceID, entry := range desired {
		if _, exists := r.state.FindAppBySource(sourceID); exists {
			continue
		}

		k := appKey{entry.Name, entry.URL}
		apps := byKey[k]
		if len(apps) == 0 {
			continue // no matching app exists, will be created normally
		}

		// Adopt the first matching app
		r.state.SetApp(apps[0].ID, sourceID)
		slog.Info("adopted existing app", "source", sourceID, "homarrID", apps[0].ID, "name", entry.Name)

		// Delete remaining duplicates
		for _, dup := range apps[1:] {
			slog.Info("deleting duplicate app", "id", dup.ID, "name", dup.Name)
			if err := r.client.DeleteApp(ctx, dup.ID); err != nil {
				slog.Error("failed to delete duplicate app", "id", dup.ID, "error", err)
				continue
			}
			deleted++
		}
		byKey[k] = nil // consumed
	}

	return deleted
}

// adoptAndDedupIntegrations matches existing Homarr integrations to desired
// entries by kind+url, adopts the first match into state, and deletes
// duplicates. Returns the number of duplicates deleted.
func (r *Reconciler) adoptAndDedupIntegrations(ctx context.Context, existing []homarr.Integration, desired map[string]source.DashboardEntry) int {
	type intKey struct{ kind, url string }

	byKey := make(map[intKey][]homarr.Integration)
	for _, intg := range existing {
		k := intKey{intg.Kind, intg.URL}
		byKey[k] = append(byKey[k], intg)
	}

	// Remove stale entries from state
	validIDs := make(map[string]bool, len(existing))
	for _, intg := range existing {
		validIDs[intg.ID] = true
	}
	for _, homarrID := range r.state.ManagedIntegrationIDs() {
		if !validIDs[homarrID] {
			r.state.RemoveIntegration(homarrID)
		}
	}

	deleted := 0
	for sourceID, entry := range desired {
		if entry.IntegrationType == "" {
			continue
		}
		if _, exists := r.state.FindIntegrationBySource(sourceID); exists {
			continue
		}

		intgURL := entry.IntegrationURL
		if intgURL == "" {
			intgURL = entry.URL
		}

		k := intKey{entry.IntegrationType, intgURL}
		intgs := byKey[k]
		if len(intgs) == 0 {
			continue
		}

		r.state.SetIntegration(intgs[0].ID, sourceID)
		slog.Info("adopted existing integration", "source", sourceID, "homarrID", intgs[0].ID, "kind", entry.IntegrationType)

		for _, dup := range intgs[1:] {
			slog.Info("deleting duplicate integration", "id", dup.ID, "kind", dup.Kind)
			if err := r.client.DeleteIntegration(ctx, dup.ID); err != nil {
				slog.Error("failed to delete duplicate integration", "id", dup.ID, "error", err)
				continue
			}
			deleted++
		}
		byKey[k] = nil
	}

	return deleted
}

// integrationIDsForSource returns the Homarr integration IDs associated with a source entry.
func (r *Reconciler) integrationIDsForSource(sourceID string) []string {
	intgID, exists := r.state.FindIntegrationBySource(sourceID)
	if !exists || intgID == "" {
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
	// Use map[string]any to handle non-string option values (booleans, nulls)
	var opts map[string]any
	if err := unmarshalJSON(item.Options, &opts); err != nil {
		return ""
	}
	if id, ok := opts["appId"].(string); ok {
		return id
	}
	return ""
}

func isFullURL(s string) bool {
	return len(s) >= 8 && (s[:7] == "http://" || s[:8] == "https://")
}
