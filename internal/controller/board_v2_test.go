package controller_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/controller"
	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/homarr"
	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/source"
)

// --- fixtures modelled on the live homarr v2.12.0 "annarchy" board ---------

const (
	layoutBase   = "layout-base"
	layoutMobile = "layout-mobile"
	rootID       = "root"
)

// v2Layouts mirrors homarr v2.12.0: getBoardByName returns layouts sorted by
// breakpoint, so the 3-column mobile layout comes first.
func v2Layouts() []homarr.Layout {
	zero := 0
	return []homarr.Layout{
		{ID: layoutMobile, Name: "Mobile", ColumnCount: 3, Breakpoint: 0, Role: "mobile", LeftGutterColumnCount: &zero, RightGutterColumnCount: &zero},
		{ID: layoutBase, Name: "Base", ColumnCount: 12, Breakpoint: 768, Role: "base", LeftGutterColumnCount: &zero, RightGutterColumnCount: &zero},
	}
}

func container(id, title string, baseY, mobileY int) homarr.Section {
	return homarr.Section{
		ID:      id,
		Kind:    homarr.SectionKindContainer,
		Options: homarr.NewContainerOptions(title),
		Layouts: []homarr.SectionLayout{
			{LayoutID: layoutMobile, ParentSectionID: rootID, XOffset: 0, YOffset: mobileY, Width: 3, Height: 1},
			{LayoutID: layoutBase, ParentSectionID: rootID, XOffset: 0, YOffset: baseY, Width: 12, Height: 1},
		},
	}
}

func rootSection() homarr.Section {
	return homarr.Section{ID: rootID, Kind: homarr.SectionKindEmpty, XOffset: 0, YOffset: 0}
}

func nonManagedApp(id, appID string, y int) homarr.BoardItem {
	return homarr.BoardItem{
		ID:              id,
		Kind:            "app",
		Options:         mustMarshal(map[string]string{"appId": appID}),
		AdvancedOptions: mustMarshal(map[string]any{"title": nil, "customCssClasses": []string{}, "borderColor": ""}),
		IntegrationIDs:  []string{},
		Layouts: []homarr.ItemLayout{
			{LayoutID: layoutMobile, SectionID: rootID, XOffset: 0, YOffset: y, Width: 1, Height: 1},
			{LayoutID: layoutBase, SectionID: rootID, XOffset: 0, YOffset: y, Width: 1, Height: 1},
		},
	}
}

// --- a port of homarr v2.12.0's save-time checks ---------------------------

// laneColumns ports getBoardLaneColumnCount for the main lane.
func laneColumns(l homarr.Layout) int {
	total := l.ColumnCount
	if total < 1 {
		total = 1
	}
	if l.Role == "mobile" {
		return total
	}
	left, right := 0, 0
	if l.LeftGutterColumnCount != nil {
		left = min(max(0, *l.LeftGutterColumnCount), total-1)
	}
	if l.RightGutterColumnCount != nil {
		right = min(max(0, *l.RightGutterColumnCount), total-left-1)
	}
	return total - left - right
}

// assertValidV2Save checks a saveBoard payload against homarr v2.12.0's
// boardSaveSchema (section union + item schema) and its
// getBoardLayoutBoundsIssue check, plus placement sanity homarr's editor
// relies on (every board layout covered, no overlaps within a grid).
func assertValidV2Save(t *testing.T, layouts []homarr.Layout, save *homarr.BoardSave) {
	t.Helper()
	if save == nil {
		t.Fatal("no board was saved")
	}

	// Schema: wire JSON of every section must match one union branch.
	raw, err := json.Marshal(save)
	if err != nil {
		t.Fatalf("marshal save: %v", err)
	}
	var wire struct {
		Sections []map[string]json.RawMessage `json:"sections"`
		Items    []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal save: %v", err)
	}
	if wire.Sections == nil || wire.Items == nil {
		t.Fatalf("sections/items must be arrays: %s", raw)
	}
	for i, s := range wire.Sections {
		switch string(s["kind"]) {
		case `"empty"`:
			if _, ok := s["xOffset"]; !ok {
				t.Errorf("sections[%d] empty without xOffset", i)
			}
			if _, ok := s["yOffset"]; !ok {
				t.Errorf("sections[%d] empty without yOffset", i)
			}
		case `"container"`:
			if l := string(s["layouts"]); l == "" || l == "null" || l[0] != '[' {
				t.Errorf("sections[%d] container without layouts array: %s", i, s["layouts"])
			}
			if _, ok := s["options"]; !ok {
				t.Errorf("sections[%d] container without options", i)
			}
		default:
			t.Errorf("sections[%d] kind %s is rejected by homarr v2.12.0 (want empty|container)", i, s["kind"])
		}
	}
	for i, it := range wire.Items {
		for _, k := range []string{"id", "kind", "options", "layouts", "integrationIds", "advancedOptions"} {
			if v, ok := it[k]; !ok || string(v) == "null" {
				t.Errorf("items[%d] missing %q", i, k)
			}
		}
	}

	// Bounds: port of getBoardLayoutBoundsIssue.
	layoutByID := map[string]homarr.Layout{}
	for _, l := range layouts {
		layoutByID[l.ID] = l
	}
	sectionByID := map[string]homarr.Section{}
	for _, s := range save.Sections {
		sectionByID[s.ID] = s
	}
	columnCount := func(layoutID, sectionID string) (int, bool) {
		l, ok := layoutByID[layoutID]
		s, ok2 := sectionByID[sectionID]
		if !ok || !ok2 {
			return 0, false
		}
		if s.Kind == homarr.SectionKindEmpty {
			return laneColumns(l), true
		}
		for _, sl := range s.Layouts {
			if sl.LayoutID == layoutID {
				return sl.Width, true
			}
		}
		return 0, false
	}
	type rect struct {
		name       string
		x, y, w, h int
	}
	grids := map[string][]rect{} // layoutID/sectionID -> occupants
	check := func(name, layoutID, sectionID string, x, y, w, h int) {
		cols, ok := columnCount(layoutID, sectionID)
		if !ok {
			t.Errorf("%s references unknown layout %q or section %q", name, layoutID, sectionID)
			return
		}
		if x < 0 || w < 1 || x+w > cols {
			t.Errorf("%s exceeds section %q bounds in layout %q: xOffset %d + width %d must fit within %d columns", name, sectionID, layoutID, x, w, cols)
		}
		if y < 0 || h < 1 {
			t.Errorf("%s has invalid y/height %d/%d", name, y, h)
		}
		key := layoutID + "/" + sectionID
		for _, o := range grids[key] {
			if x < o.x+o.w && o.x < x+w && y < o.y+o.h && o.y < y+h {
				t.Errorf("%s overlaps %s in %s", name, o.name, key)
			}
		}
		grids[key] = append(grids[key], rect{name, x, y, w, h})
	}
	for _, s := range save.Sections {
		if s.Kind != homarr.SectionKindContainer {
			continue
		}
		seen := map[string]bool{}
		for _, sl := range s.Layouts {
			seen[sl.LayoutID] = true
			check(fmt.Sprintf("Section %q", s.ID), sl.LayoutID, sl.ParentSectionID, sl.XOffset, sl.YOffset, sl.Width, sl.Height)
		}
		for _, l := range layouts {
			if !seen[l.ID] {
				t.Errorf("container %q has no placement in layout %q", s.ID, l.Name)
			}
		}
	}
	for _, it := range save.Items {
		seen := map[string]bool{}
		for _, il := range it.Layouts {
			seen[il.LayoutID] = true
			check(fmt.Sprintf("Item %q", it.ID), il.LayoutID, il.SectionID, il.XOffset, il.YOffset, il.Width, il.Height)
		}
		for _, l := range layouts {
			if !seen[l.ID] {
				t.Errorf("item %q has no placement in layout %q", it.ID, l.Name)
			}
		}
	}

	// A container must be tall enough for its content in each layout.
	for _, s := range save.Sections {
		if s.Kind != homarr.SectionKindContainer {
			continue
		}
		for _, sl := range s.Layouts {
			bottom := 0
			for _, o := range grids[sl.LayoutID+"/"+s.ID] {
				bottom = max(bottom, o.y+o.h)
			}
			if sl.Height < bottom {
				t.Errorf("container %q height %d in layout %q is shorter than its content (%d rows)", s.ID, sl.Height, sl.LayoutID, bottom)
			}
		}
	}
}

// sectionOfApp returns the sectionID an app's managed item uses per layout.
func sectionOfApp(t *testing.T, save *homarr.BoardSave, appID string) map[string]string {
	t.Helper()
	for _, it := range save.Items {
		var opts map[string]any
		if err := json.Unmarshal(it.Options, &opts); err != nil {
			continue
		}
		if opts["appId"] != appID {
			continue
		}
		out := map[string]string{}
		for _, l := range it.Layouts {
			out[l.LayoutID] = l.SectionID
		}
		return out
	}
	t.Fatalf("app %q not on saved board", appID)
	return nil
}

func containerTitles(save *homarr.BoardSave) []string {
	var out []string
	for _, s := range save.Sections {
		if s.IsContainer() {
			out = append(out, s.Title())
		}
	}
	sort.Strings(out)
	return out
}

// boardFromSave turns a saved payload back into the board homarr would
// return on the next getBoardByName, so a second reconcile sees it.
func boardFromSave(id, name string, layouts []homarr.Layout, save *homarr.BoardSave) *homarr.Board {
	return &homarr.Board{ID: id, Name: name, Layouts: layouts, Sections: save.Sections, Items: save.Items}
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// --- tests -----------------------------------------------------------------

func TestReconciler_PlacesAppsInExistingContainerSectionsByTitle(t *testing.T) {
	mock := &mockHomarrClient{
		boardExists: true,
		apps: []homarr.App{
			{ID: "app-flux", Name: "Flux", Href: "https://flux.example.com"},
			{ID: "app-sonarr", Name: "Sonarr", Href: "https://sonarr.example.com"},
			{ID: "app-akkoma", Name: "Akkoma", Href: "https://akkoma.example.com"},
		},
		board: &homarr.Board{
			ID: "board-1", Name: "test", Layouts: v2Layouts(),
			Sections: []homarr.Section{rootSection(), container("c-infra", "Infra", 0, 0), container("c-services", "Services", 2, 5)},
		},
	}
	src := &mockSource{entries: []source.DashboardEntry{
		{ID: "flux-system/HTTPRoute/flux", Name: "Flux", URL: "https://flux.example.com", Category: "Infra"},
		{ID: "htpc/Ingress/sonarr", Name: "Sonarr", URL: "https://sonarr.example.com", Category: "Services"},
		{ID: "akkoma/Ingress/akkoma", Name: "Akkoma", URL: "https://akkoma.example.com", Category: "Services"},
	}}
	logs := captureLogs(t)

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "test", 12, "")
	if _, err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	save := mock.savedBoard
	assertValidV2Save(t, v2Layouts(), save)

	if got := len(save.Sections); got != 3 {
		t.Errorf("sections = %d, want 3 (existing containers reused, none added)", got)
	}
	if strings.Contains(logs.String(), "created category section") {
		t.Errorf("existing container sections were re-created:\n%s", logs.String())
	}
	for app, want := range map[string]string{"app-flux": "c-infra", "app-sonarr": "c-services", "app-akkoma": "c-services"} {
		for layout, got := range sectionOfApp(t, save, app) {
			if got != want {
				t.Errorf("%s in layout %s placed in section %q, want %q", app, layout, got, want)
			}
		}
	}
}

func TestReconciler_CreatesMissingCategoryAsContainerSection(t *testing.T) {
	mock := &mockHomarrClient{
		boardExists: true,
		board:       &homarr.Board{ID: "board-1", Name: "test", Layouts: v2Layouts(), Sections: []homarr.Section{rootSection()}},
	}
	src := &mockSource{entries: []source.DashboardEntry{
		{ID: "htpc/Ingress/sonarr", Name: "Sonarr", URL: "https://sonarr.example.com", Category: "Services"},
		{ID: "htpc/Ingress/radarr", Name: "Radarr", URL: "https://radarr.example.com", Category: "Services"},
		{ID: "htpc/Ingress/lidarr", Name: "Lidarr", URL: "https://lidarr.example.com", Category: "Services"},
		{ID: "htpc/Ingress/readarr", Name: "Readarr", URL: "https://readarr.example.com", Category: "Services"},
		{ID: "default/Ingress/misc", Name: "Misc", URL: "https://misc.example.com"},
	}}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "test", 12, "")
	if _, err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	save := mock.savedBoard
	assertValidV2Save(t, v2Layouts(), save)

	if got := containerTitles(save); len(got) != 1 || got[0] != "Services" {
		t.Fatalf("container titles = %v, want [Services]", got)
	}
	var svc homarr.Section
	for _, s := range save.Sections {
		if s.IsContainer() {
			svc = s
		}
	}
	if svc.Kind != homarr.SectionKindContainer {
		t.Errorf("new section kind = %q, want container", svc.Kind)
	}
	for _, sl := range svc.Layouts {
		if sl.ParentSectionID != rootID {
			t.Errorf("container placed under %q in %s, want root section", sl.ParentSectionID, sl.LayoutID)
		}
		wantWidth := map[string]int{layoutBase: 12, layoutMobile: 3}[sl.LayoutID]
		if sl.Width != wantWidth {
			t.Errorf("container width in %s = %d, want full lane %d", sl.LayoutID, sl.Width, wantWidth)
		}
	}
	// 4 apps in a 3-column mobile container need 2 rows.
	for _, sl := range svc.Layouts {
		if sl.LayoutID == layoutMobile && sl.Height != 2 {
			t.Errorf("mobile container height = %d, want 2", sl.Height)
		}
		if sl.LayoutID == layoutBase && sl.Height != 1 {
			t.Errorf("base container height = %d, want 1", sl.Height)
		}
	}
	for layout, got := range sectionOfApp(t, save, "new-Misc") {
		if got != rootID {
			t.Errorf("uncategorised app in layout %s placed in %q, want root", layout, got)
		}
	}
	for layout, got := range sectionOfApp(t, save, "new-Sonarr") {
		if got != svc.ID {
			t.Errorf("Sonarr in layout %s placed in %q, want %q", layout, got, svc.ID)
		}
	}
}

func TestReconciler_SecondReconcileDoesNotRecreateSections(t *testing.T) {
	// AC: while the board already has the controller's category sections, a
	// reconcile does not create them again and the section count is stable.
	mock := &mockHomarrClient{
		boardExists: true,
		board:       &homarr.Board{ID: "board-1", Name: "test", Layouts: v2Layouts(), Sections: []homarr.Section{rootSection()}},
	}
	src := &mockSource{entries: []source.DashboardEntry{
		{ID: "htpc/Ingress/sonarr", Name: "Sonarr", URL: "https://sonarr.example.com", Category: "Services"},
		{ID: "kube-system/Ingress/hubble", Name: "Hubble", URL: "https://hubble.example.com", Category: "Infra"},
	}}
	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "test", 12, "")
	if _, err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	first := mock.savedBoard
	assertValidV2Save(t, v2Layouts(), first)
	if got := len(first.Sections); got != 3 {
		t.Fatalf("first reconcile sections = %d, want 3 (root + Infra + Services)", got)
	}

	// homarr now returns what was saved; apps exist in homarr.
	mock.board = boardFromSave("board-1", "test", v2Layouts(), first)
	mock.apps = []homarr.App{
		{ID: "new-Sonarr", Name: "Sonarr", Href: "https://sonarr.example.com"},
		{ID: "new-Hubble", Name: "Hubble", Href: "https://hubble.example.com"},
	}
	logs := captureLogs(t)

	for i := 0; i < 2; i++ {
		if _, err := r.Reconcile(context.Background()); err != nil {
			t.Fatalf("Reconcile %d: %v", i+2, err)
		}
		assertValidV2Save(t, v2Layouts(), mock.savedBoard)
		if got := len(mock.savedBoard.Sections); got != 3 {
			t.Errorf("reconcile %d sections = %d, want 3", i+2, got)
		}
		mock.board = boardFromSave("board-1", "test", v2Layouts(), mock.savedBoard)
	}
	if strings.Contains(logs.String(), "created category section") {
		t.Errorf("no-change reconcile logged a section creation:\n%s", logs.String())
	}
	firstJSON, _ := json.Marshal(first.Sections)
	lastJSON, _ := json.Marshal(mock.savedBoard.Sections)
	if !bytes.Equal(firstJSON, lastJSON) {
		t.Errorf("sections drifted across no-change reconciles:\nfirst: %s\nlast:  %s", firstJSON, lastJSON)
	}
}

func TestReconciler_MovesManagedAppToItsCategoryContainer(t *testing.T) {
	stale := homarr.BoardItem{
		ID: "managed-app-flux", Kind: "app",
		Options:         mustMarshal(map[string]string{"appId": "app-flux"}),
		AdvancedOptions: mustMarshal(map[string]any{"title": nil, "customCssClasses": []string{}, "borderColor": ""}),
		IntegrationIDs:  []string{},
		Layouts: []homarr.ItemLayout{
			{LayoutID: layoutMobile, SectionID: rootID, XOffset: 0, YOffset: 0, Width: 1, Height: 1},
			{LayoutID: layoutBase, SectionID: rootID, XOffset: 0, YOffset: 0, Width: 1, Height: 1},
		},
	}
	mock := &mockHomarrClient{
		boardExists: true,
		apps:        []homarr.App{{ID: "app-flux", Name: "Flux", Href: "https://flux.example.com"}},
		board: &homarr.Board{
			ID: "board-1", Name: "test", Layouts: v2Layouts(),
			Sections: []homarr.Section{rootSection(), container("c-infra", "Infra", 0, 0)},
			Items:    []homarr.BoardItem{stale},
		},
	}
	src := &mockSource{entries: []source.DashboardEntry{
		{ID: "flux-system/HTTPRoute/flux", Name: "Flux", URL: "https://flux.example.com", Category: "Infra"},
	}}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "test", 12, "")
	if _, err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	assertValidV2Save(t, v2Layouts(), mock.savedBoard)
	for layout, got := range sectionOfApp(t, mock.savedBoard, "app-flux") {
		if got != "c-infra" {
			t.Errorf("Flux in layout %s = %q, want c-infra", layout, got)
		}
	}
	n := 0
	for _, it := range mock.savedBoard.Items {
		if it.ID == "managed-app-flux" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("managed-app-flux appears %d times, want 1 (same id keeps it an update)", n)
	}
}

func TestReconciler_PreservesNonManagedItemsAndContainers(t *testing.T) {
	// The live board has 3 non-managed items and may have user containers
	// whose titles are not controller categories; both must survive a save.
	userContainer := container("c-user", "Bookmarks", 0, 0)
	userContainer.Options = json.RawMessage(`{"title":"Bookmarks","customCssClasses":["x"],"borderColor":"#ff0000","showLabel":false,"collapsible":false,"showOpenAll":false,"scrollable":true}`)
	mock := &mockHomarrClient{
		boardExists: true,
		apps: []homarr.App{
			{ID: "app-sonarr", Name: "Sonarr", Href: "https://sonarr.example.com"},
			{ID: "user-app-1", Name: "Bank", Href: "https://bank.example.com"},
		},
		board: &homarr.Board{
			ID: "board-1", Name: "test", Layouts: v2Layouts(),
			Sections: []homarr.Section{rootSection(), userContainer},
			Items: []homarr.BoardItem{
				nonManagedApp("u1", "user-app-1", 1),
				nonManagedApp("u2", "user-app-2", 2),
				nonManagedApp("u3", "user-app-3", 3),
			},
		},
	}
	src := &mockSource{entries: []source.DashboardEntry{
		{ID: "htpc/Ingress/sonarr", Name: "Sonarr", URL: "https://sonarr.example.com", Category: "Services"},
	}}

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "test", 12, "")
	if _, err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	save := mock.savedBoard
	assertValidV2Save(t, v2Layouts(), save)

	byID := map[string]homarr.BoardItem{}
	for _, it := range save.Items {
		byID[it.ID] = it
	}
	for _, id := range []string{"u1", "u2", "u3"} {
		it, ok := byID[id]
		if !ok {
			t.Errorf("non-managed item %s dropped", id)
			continue
		}
		for _, l := range it.Layouts {
			if l.SectionID != rootID || l.XOffset != 0 {
				t.Errorf("non-managed item %s moved: %+v", id, l)
			}
		}
	}
	var kept homarr.Section
	for _, s := range save.Sections {
		if s.ID == "c-user" {
			kept = s
		}
	}
	if kept.ID == "" {
		t.Fatal("user container dropped")
	}
	if !bytes.Equal(kept.Options, userContainer.Options) {
		t.Errorf("user container options changed: %s", kept.Options)
	}
	for i, sl := range kept.Layouts {
		if sl != userContainer.Layouts[i] {
			t.Errorf("user container placement changed: %+v -> %+v", userContainer.Layouts[i], sl)
		}
	}
	if _, ok := byID["managed-app-sonarr"]; !ok {
		t.Error("managed Sonarr item missing")
	}
}

func TestReconciler_RespectsGutterLaneWidth(t *testing.T) {
	two := 2
	layouts := v2Layouts()
	layouts[1].LeftGutterColumnCount = &two // base: 12 - 2 = 10 main columns
	mock := &mockHomarrClient{
		boardExists: true,
		board:       &homarr.Board{ID: "board-1", Name: "test", Layouts: layouts, Sections: []homarr.Section{rootSection()}},
	}
	var entries []source.DashboardEntry
	for i := 0; i < 11; i++ {
		entries = append(entries, source.DashboardEntry{
			ID: fmt.Sprintf("ns/Ingress/app%02d", i), Name: fmt.Sprintf("App%02d", i), URL: fmt.Sprintf("https://app%02d.example.com", i),
		})
	}
	r := controller.NewReconciler(mock, []controller.SourceLister{&mockSource{entries: entries}}, "test", 12, "")
	if _, err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	assertValidV2Save(t, layouts, mock.savedBoard)
}

func TestReconciler_FailedIntegrationCreateIsNotCountedOrLinked(t *testing.T) {
	// AC: the controller shall not log "created integration" or count
	// integrations_created when homarr returned no id; a failed create is
	// logged as an error with homarr's response.
	homarrResponse := `{"error":{"type":"request","data":{"type":"timeout"}}}`
	mock := &mockHomarrClient{
		boardExists:          true,
		createIntegrationErr: errors.New("integration.create for plex failed: " + homarrResponse),
	}
	src := &mockSource{entries: []source.DashboardEntry{{
		ID: "htpc/HTTPRoute/plex", Name: "Plex", URL: "https://plex.example.com",
		IntegrationType: "plex", IntegrationURL: "http://plex-tcp.htpc.svc.cluster.local",
		IntegrationSecret: "plex-api-key", IntegrationSecretKey: "api-key",
	}}}
	logs := captureLogs(t)

	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "test", 12, "")
	r.SetSecretReader(&mockSecretReader{secrets: map[string]map[string][]byte{
		"htpc/plex-api-key": {"api-key": []byte("token")},
	}})
	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.IntegrationsCreated != 0 {
		t.Errorf("IntegrationsCreated = %d, want 0", result.IntegrationsCreated)
	}
	if _, ok := r.State().FindIntegrationBySource("htpc/HTTPRoute/plex"); ok {
		t.Error("failed integration recorded in state")
	}
	out := logs.String()
	if strings.Contains(out, "created integration") {
		t.Errorf("logged success for a failed create:\n%s", out)
	}
	if !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "failed to create integration") || !strings.Contains(out, `data`) {
		t.Errorf("failed create not logged as an error with homarr's response:\n%s", out)
	}
	for _, it := range mock.savedBoard.Items {
		if len(it.IntegrationIDs) != 0 {
			t.Errorf("item %s linked to integrations %v after a failed create", it.ID, it.IntegrationIDs)
		}
	}
}

func TestReconciler_SuccessfulSaveIsReported(t *testing.T) {
	mock := &mockHomarrClient{boardExists: true}
	src := &mockSource{entries: []source.DashboardEntry{
		{ID: "htpc/Ingress/sonarr", Name: "Sonarr", URL: "https://sonarr.example.com"},
	}}
	r := controller.NewReconciler(mock, []controller.SourceLister{src}, "test", 12, "")
	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !result.BoardSaved {
		t.Error("BoardSaved = false after a successful saveBoard")
	}
	if mock.saveCalls != 1 {
		t.Errorf("saveCalls = %d", mock.saveCalls)
	}
}
