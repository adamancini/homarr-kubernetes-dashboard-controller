package controller

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/homarr"
	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/source"
)

// Board model (homarr v2.12.0):
//
//   - Root sections are kind "empty", one per lane (left/main/right, chosen by
//     xOffset -1/0/1). The main root spans the layout's main-lane columns.
//   - A category is a "container" section titled with the category name
//     (options.title). Containers sit inside a parent section with one
//     placement per board layout and have their own grid as wide as that
//     placement.
//   - Every item carries one placement per board layout.
//
// The controller owns the items it creates ("managed-<appID>") and the
// containers whose titles are categories of managed apps. Everything else on
// the board is left where it is and only avoided.

// containerTitleMaxLen mirrors homarr's 64 character limit on container titles.
const containerTitleMaxLen = 64

// rect is an occupied grid area within one section of one layout.
type rect struct{ x, y, w, h int }

func (a rect) overlaps(b rect) bool {
	return a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h
}

// grid tracks occupied areas in one section of one layout.
type grid []rect

func (g grid) bottom() int {
	b := 0
	for _, r := range g {
		b = max(b, r.y+r.h)
	}
	return b
}

// firstFreeRow returns the first row at or below y where a w×h area at column
// x does not overlap anything in the grid.
func (g grid) firstFreeRow(x, y, w, h int) int {
	for {
		candidate := rect{x, y, w, h}
		moved := false
		for _, r := range g {
			if candidate.overlaps(r) {
				y = r.y + r.h
				moved = true
				break
			}
		}
		if !moved {
			return y
		}
	}
}

// categoryKey normalises a category name to the title homarr can store.
func categoryKey(category string) string {
	if len(category) > containerTitleMaxLen {
		return category[:containerTitleMaxLen]
	}
	return category
}

// mainRootSection returns the "empty" root section of the main lane (lowest
// yOffset first). Items without a category are placed there and new category
// containers are created inside it.
func mainRootSection(sections []homarr.Section) (homarr.Section, bool) {
	var best homarr.Section
	found := false
	for _, s := range sections {
		if s.Kind != homarr.SectionKindEmpty || s.XOffset == -1 || s.XOffset == 1 {
			continue
		}
		if !found || s.YOffset < best.YOffset {
			best, found = s, true
		}
	}
	return best, found
}

// buildCategoryContainerMap maps container titles to section indexes. The
// first container with a given title wins.
func buildCategoryContainerMap(sections []homarr.Section) map[string]int {
	result := make(map[string]int)
	for i, s := range sections {
		if !s.IsContainer() {
			continue
		}
		title := s.Title()
		if title == "" {
			continue
		}
		if _, exists := result[title]; !exists {
			result[title] = i
		}
	}
	return result
}

func sectionLayoutFor(s homarr.Section, layoutID string) (homarr.SectionLayout, int) {
	for i, l := range s.Layouts {
		if l.LayoutID == layoutID {
			return l, i
		}
	}
	return homarr.SectionLayout{}, -1
}

// placeAppsOnBoard places every managed app on the board, inside the
// container section of its category (created when missing), with one
// placement per board layout, and saves the board. It returns whether a save
// happened.
func (r *Reconciler) placeAppsOnBoard(ctx context.Context, board homarr.Board, desired map[string]source.DashboardEntry) (bool, error) {
	if len(board.Layouts) == 0 {
		return false, fmt.Errorf("board %q has no layouts", board.Name)
	}
	root, ok := mainRootSection(board.Sections)
	if !ok {
		return false, fmt.Errorf("board %q has no main root section", board.Name)
	}

	sections := make([]homarr.Section, len(board.Sections))
	copy(sections, board.Sections)
	for i := range sections { // placements are rewritten below; never alias the caller's board
		sections[i].Layouts = append([]homarr.SectionLayout(nil), sections[i].Layouts...)
	}
	containerIdx := buildCategoryContainerMap(sections)

	// Keep non-managed items and note which apps they already show.
	onBoard := make(map[string]bool)
	var items []homarr.BoardItem
	for _, item := range board.Items {
		if item.Kind == "app" && strings.HasPrefix(item.ID, "managed-") {
			continue // managed items are rebuilt below (same ids, so homarr updates them)
		}
		items = append(items, item)
		if item.Kind == "app" {
			if id := appIDFromItem(item); id != "" {
				onBoard[id] = true
			}
		}
	}
	baseLen := len(items)

	// Managed apps to place, grouped by target category ("" = root).
	appsByCategory := make(map[string][]string)
	var toPlace []string
	for _, appID := range r.state.ManagedAppIDs() {
		if onBoard[appID] {
			continue
		}
		cat := ""
		if entry, ok := desired[r.state.GetAppSource(appID)]; ok {
			cat = categoryKey(entry.Category)
		}
		appsByCategory[cat] = append(appsByCategory[cat], appID)
		toPlace = append(toPlace, appID)
	}

	// Create containers for categories that have none.
	var categories []string
	for cat := range appsByCategory {
		if cat != "" {
			categories = append(categories, cat)
		}
	}
	sort.Strings(categories)
	for _, cat := range categories {
		if _, exists := containerIdx[cat]; exists {
			continue
		}
		sections = append(sections, homarr.Section{
			ID:      generateSectionID("cat", cat),
			Kind:    homarr.SectionKindContainer,
			Options: homarr.NewContainerOptions(cat),
		})
		containerIdx[cat] = len(sections) - 1
		slog.Info("created category section", "category", cat)
	}

	// Section each managed container belongs to, and the reverse.
	managedContainer := make(map[string]string) // sectionID -> category
	for _, cat := range categories {
		managedContainer[sections[containerIdx[cat]].ID] = cat
	}

	// itemPlacements[appID] collects one placement per layout.
	itemPlacements := make(map[string][]homarr.ItemLayout, len(toPlace))

	for _, layout := range board.Layouts {
		laneCols := layout.MainLaneColumnCount()

		// Occupied areas of things the controller does not move.
		fixed := make(map[string]grid) // sectionID -> occupied
		for _, item := range items {
			for _, l := range item.Layouts {
				if l.LayoutID == layout.ID {
					fixed[l.SectionID] = append(fixed[l.SectionID], rect{l.XOffset, l.YOffset, l.Width, l.Height})
				}
			}
		}
		for _, s := range sections {
			if !s.IsContainer() {
				continue
			}
			if _, managed := managedContainer[s.ID]; managed {
				continue
			}
			if sl, i := sectionLayoutFor(s, layout.ID); i >= 0 {
				fixed[sl.ParentSectionID] = append(fixed[sl.ParentSectionID], rect{sl.XOffset, sl.YOffset, sl.Width, sl.Height})
			}
		}

		// Order managed containers: placed ones by their current row, then new ones by title.
		type pending struct {
			idx      int
			existing bool
			sl       homarr.SectionLayout
		}
		var order []pending
		for _, cat := range categories {
			idx := containerIdx[cat]
			sl, i := sectionLayoutFor(sections[idx], layout.ID)
			order = append(order, pending{idx: idx, existing: i >= 0, sl: sl})
		}
		sort.SliceStable(order, func(a, b int) bool {
			if order[a].existing != order[b].existing {
				return order[a].existing
			}
			if order[a].existing && order[a].sl.YOffset != order[b].sl.YOffset {
				return order[a].sl.YOffset < order[b].sl.YOffset
			}
			return false // keep title order from categories
		})

		rootGrid := append(grid{}, fixed[root.ID]...)
		for _, p := range order {
			s := &sections[p.idx]
			cat := managedContainer[s.ID]
			sl := p.sl
			if !p.existing {
				sl = homarr.SectionLayout{LayoutID: layout.ID, ParentSectionID: root.ID, XOffset: 0, Width: laneCols}
				sl.YOffset = rootGrid.bottom()
			}
			parentIsRoot := sl.ParentSectionID == root.ID
			if parentIsRoot && (sl.Width < 1 || sl.XOffset < 0 || sl.XOffset+sl.Width > laneCols) {
				sl.XOffset, sl.Width = 0, laneCols
			}
			width := max(1, sl.Width)

			// Place this category's apps inside the container, below any
			// non-managed content it already holds.
			inner := fixed[s.ID]
			col, row := 0, inner.bottom()
			for _, appID := range appsByCategory[cat] {
				if col >= width {
					col, row = 0, row+1
				}
				row = inner.firstFreeRow(col, row, 1, 1)
				itemPlacements[appID] = append(itemPlacements[appID], homarr.ItemLayout{
					LayoutID: layout.ID, SectionID: s.ID, XOffset: col, YOffset: row, Width: 1, Height: 1,
				})
				inner = append(inner, rect{col, row, 1, 1})
				col++
			}
			sl.Height = max(1, inner.bottom())

			if parentIsRoot {
				sl.YOffset = rootGrid.firstFreeRow(sl.XOffset, sl.YOffset, sl.Width, sl.Height)
				rootGrid = append(rootGrid, rect{sl.XOffset, sl.YOffset, sl.Width, sl.Height})
			}
			if _, i := sectionLayoutFor(*s, layout.ID); i >= 0 {
				s.Layouts[i] = sl
			} else {
				s.Layouts = append(s.Layouts, sl)
			}
		}

		// Uncategorised managed apps go into the root, below everything else.
		col, row := 0, rootGrid.bottom()
		for _, appID := range appsByCategory[""] {
			if col >= laneCols {
				col, row = 0, row+1
			}
			itemPlacements[appID] = append(itemPlacements[appID], homarr.ItemLayout{
				LayoutID: layout.ID, SectionID: root.ID, XOffset: col, YOffset: row, Width: 1, Height: 1,
			})
			col++
		}
	}

	for _, appID := range toPlace {
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
			IntegrationIDs: r.integrationIDsForSource(r.state.GetAppSource(appID)),
			Layouts:        itemPlacements[appID],
		})
	}

	managedCount := len(items) - baseLen
	oldManagedCount := len(board.Items) - baseLen
	slog.Info("board item audit",
		"nonManaged", baseLen, "managed", managedCount,
		"previousManaged", oldManagedCount, "total", len(items))

	if managedCount == 0 && oldManagedCount == 0 && len(sections) == len(board.Sections) {
		slog.Info("board placement: no changes needed")
		return false, nil
	}

	slog.Info("saving board", "items", len(items), "sections", len(sections))
	if err := r.client.SaveBoard(ctx, homarr.BoardSave{
		ID:       board.ID,
		Sections: sections,
		Items:    items,
	}); err != nil {
		return false, err
	}
	slog.Info("saved board", "board", board.Name, "items", len(items), "sections", len(sections))
	return true, nil
}
