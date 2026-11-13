package homarr_test

import (
	"encoding/json"
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/homarr"
)

// These tests pin the section wire format to homarr v2.12.0's
// boardSaveSchema (packages/validation/src/shared.ts sectionSchema):
//
//	empty:     {id, kind: "empty", xOffset: int, yOffset: int}
//	container: {id, kind: "container", options, collapsed, layouts: [{layoutId,
//	            xOffset, yOffset, width, height, parentSectionId}]}
//	dynamic:   legacy alias of container (same shape)
//
// Any other kind (for example the pre-v2 "category") or a container without a
// layouts array is rejected with 400 invalid_union.

func marshalSection(t *testing.T, s homarr.Section) map[string]json.RawMessage {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal section: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal section: %v", err)
	}
	return m
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestSection_MarshalEmpty_V2Shape(t *testing.T) {
	m := marshalSection(t, homarr.Section{ID: "root", Kind: homarr.SectionKindEmpty, XOffset: 0, YOffset: 0})

	want := map[string]string{"id": `"root"`, "kind": `"empty"`, "xOffset": `0`, "yOffset": `0`}
	if len(m) != len(want) {
		t.Fatalf("empty section keys = %v, want exactly %v", keys(m), want)
	}
	for k, v := range want {
		if string(m[k]) != v {
			t.Errorf("empty section %s = %s, want %s", k, m[k], v)
		}
	}
}

func TestSection_MarshalContainer_V2Shape(t *testing.T) {
	collapsed := true
	s := homarr.Section{
		ID:        "c1",
		Kind:      homarr.SectionKindContainer,
		Name:      "ignored-legacy-name",
		XOffset:   3,
		YOffset:   4,
		Collapsed: &collapsed,
		Options:   json.RawMessage(`{"title":"Infra","showLabel":true}`),
		Layouts: []homarr.SectionLayout{
			{LayoutID: "base", XOffset: 0, YOffset: 2, Width: 12, Height: 1, ParentSectionID: "root"},
		},
	}
	m := marshalSection(t, s)

	if string(m["kind"]) != `"container"` {
		t.Errorf("kind = %s, want \"container\"", m["kind"])
	}
	for _, k := range []string{"xOffset", "yOffset", "name"} {
		if _, ok := m[k]; ok {
			t.Errorf("container section must not carry %q (homarr strips it; root offsets are for empty sections only)", k)
		}
	}
	if string(m["collapsed"]) != "true" {
		t.Errorf("collapsed = %s, want true", m["collapsed"])
	}
	var opts map[string]any
	if err := json.Unmarshal(m["options"], &opts); err != nil || opts["title"] != "Infra" {
		t.Errorf("options = %s, want title Infra (err %v)", m["options"], err)
	}
	var layouts []map[string]any
	if err := json.Unmarshal(m["layouts"], &layouts); err != nil {
		t.Fatalf("layouts = %s: %v", m["layouts"], err)
	}
	if len(layouts) != 1 {
		t.Fatalf("layouts len = %d, want 1", len(layouts))
	}
	l := layouts[0]
	if l["layoutId"] != "base" || l["parentSectionId"] != "root" || l["width"] != float64(12) ||
		l["height"] != float64(1) || l["xOffset"] != float64(0) || l["yOffset"] != float64(2) {
		t.Errorf("layout = %v, want layoutId=base parentSectionId=root x=0 y=2 w=12 h=1", l)
	}
}

func TestSection_MarshalContainer_AlwaysSendsLayoutsArrayAndCollapsed(t *testing.T) {
	// homarr's container branch requires layouts to be an array; null or a
	// missing key is "expected array, received undefined".
	m := marshalSection(t, homarr.Section{ID: "c1", Kind: homarr.SectionKindContainer})

	if string(m["layouts"]) != "[]" {
		t.Errorf("layouts = %s, want []", m["layouts"])
	}
	if string(m["collapsed"]) != "false" {
		t.Errorf("collapsed = %s, want false", m["collapsed"])
	}
	if _, ok := m["options"]; !ok {
		t.Error("container section must carry options")
	}
}

func TestSection_MarshalLegacyDynamic_NormalisedToContainer(t *testing.T) {
	m := marshalSection(t, homarr.Section{
		ID:      "d1",
		Kind:    "dynamic",
		Options: json.RawMessage(`{"title":""}`),
		Layouts: []homarr.SectionLayout{{LayoutID: "base", Width: 4, Height: 2, ParentSectionID: "root"}},
	})
	if string(m["kind"]) != `"container"` {
		t.Errorf("kind = %s, want \"container\"", m["kind"])
	}
	if _, ok := m["layouts"]; !ok {
		t.Error("dynamic section lost its layouts")
	}
}

func TestSection_UnmarshalV2BoardResponse(t *testing.T) {
	// Shape returned by homarr v2.12.0 board.getBoardByName (as stored for the
	// live "annarchy" board): containers have null offsets and name.
	raw := `[
		{"id":"root","kind":"empty","xOffset":0,"yOffset":0,"name":null,"options":{},"layouts":[],"collapsed":false},
		{"id":"c-infra","kind":"container","xOffset":null,"yOffset":null,"name":null,
		 "options":{"title":"Infra","customCssClasses":[],"borderColor":"","showLabel":true,"collapsible":true,"showOpenAll":true,"scrollable":false},
		 "layouts":[{"xOffset":0,"yOffset":0,"width":12,"height":1,"parentSectionId":"root","layoutId":"base"},
		            {"xOffset":0,"yOffset":0,"width":3,"height":1,"parentSectionId":"root","layoutId":"mobile"}],
		 "collapsed":false}
	]`
	var sections []homarr.Section
	if err := json.Unmarshal([]byte(raw), &sections); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(sections) != 2 {
		t.Fatalf("got %d sections", len(sections))
	}
	if sections[0].IsContainer() {
		t.Error("empty section reported as container")
	}
	c := sections[1]
	if !c.IsContainer() {
		t.Fatal("container section not reported as container")
	}
	if got := c.Title(); got != "Infra" {
		t.Errorf("Title() = %q, want Infra", got)
	}
	if len(c.Layouts) != 2 || c.Layouts[1].LayoutID != "mobile" || c.Layouts[1].Width != 3 || c.Layouts[1].ParentSectionID != "root" {
		t.Errorf("layouts = %+v", c.Layouts)
	}
}

func TestSection_TitleFallsBackToLegacyName(t *testing.T) {
	s := homarr.Section{ID: "x", Kind: "dynamic", Name: "Legacy"}
	if got := s.Title(); got != "Legacy" {
		t.Errorf("Title() = %q, want Legacy", got)
	}
}

func TestNewContainerOptions_ValidForHomarr(t *testing.T) {
	var opts map[string]any
	if err := json.Unmarshal(homarr.NewContainerOptions("Services"), &opts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if opts["title"] != "Services" {
		t.Errorf("title = %v", opts["title"])
	}
	for _, k := range []string{"customCssClasses", "borderColor", "showLabel", "collapsible", "showOpenAll", "scrollable"} {
		if _, ok := opts[k]; !ok {
			t.Errorf("container options missing %q", k)
		}
	}
	if opts["showLabel"] != true {
		t.Error("showLabel must be true so the category title is visible")
	}
}

func TestNewContainerOptions_TruncatesTitleTo64(t *testing.T) {
	long := ""
	for i := 0; i < 80; i++ {
		long += "x"
	}
	var opts map[string]any
	if err := json.Unmarshal(homarr.NewContainerOptions(long), &opts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := opts["title"].(string); len(got) != 64 {
		t.Errorf("title length = %d, want 64 (homarr max)", len(got))
	}
}

func TestLayout_MainLaneColumnCount(t *testing.T) {
	left, right := 2, 1
	cases := []struct {
		name   string
		layout homarr.Layout
		want   int
	}{
		{"no gutters", homarr.Layout{ColumnCount: 12, Role: "base"}, 12},
		{"gutters on base", homarr.Layout{ColumnCount: 12, Role: "base", LeftGutterColumnCount: &left, RightGutterColumnCount: &right}, 9},
		{"mobile ignores gutters", homarr.Layout{ColumnCount: 3, Role: "mobile", LeftGutterColumnCount: &left, RightGutterColumnCount: &right}, 3},
		{"zero columns clamps to 1", homarr.Layout{ColumnCount: 0}, 1},
	}
	for _, tc := range cases {
		if got := tc.layout.MainLaneColumnCount(); got != tc.want {
			t.Errorf("%s: MainLaneColumnCount() = %d, want %d", tc.name, got, tc.want)
		}
	}
}
