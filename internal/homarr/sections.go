package homarr

import "encoding/json"

// Section kinds accepted by homarr v2.12.0's boardSaveSchema
// (packages/validation/src/shared.ts sectionSchema). "dynamic" is a legacy
// alias homarr still accepts and rewrites to "container". The pre-v2
// "category" kind is gone: homarr migration 0004_unify_sections_and_gutters
// turned every category into a container whose options.title holds the old
// category name.
const (
	SectionKindEmpty         = "empty"
	SectionKindContainer     = "container"
	sectionKindLegacyDynamic = "dynamic"

	// containerTitleMaxLen is the max length of a container title in homarr.
	containerTitleMaxLen = 64
)

// Section is a board section as returned by board.getBoardByName.
//
// Root sections are kind "empty" and are positioned by XOffset (lane) and
// YOffset. Container sections are kind "container" and are positioned inside
// a parent section with one SectionLayout per board layout.
type Section struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Name      string          `json:"name,omitempty"` // legacy; homarr v2 stores null
	XOffset   int             `json:"xOffset"`
	YOffset   int             `json:"yOffset"`
	Collapsed *bool           `json:"collapsed,omitempty"`
	Options   json.RawMessage `json:"options,omitempty"`
	Layouts   []SectionLayout `json:"layouts,omitempty"`
}

// SectionLayout places a container section inside its parent section for
// one board layout.
type SectionLayout struct {
	LayoutID        string `json:"layoutId"`
	XOffset         int    `json:"xOffset"`
	YOffset         int    `json:"yOffset"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	ParentSectionID string `json:"parentSectionId"`
}

// containerOptions is the subset of homarr's containerSectionOptionsSchema
// the controller reads.
type containerOptions struct {
	Title string `json:"title"`
}

// IsContainer reports whether the section is a container (including the
// legacy "dynamic" alias).
func (s Section) IsContainer() bool {
	return s.Kind == SectionKindContainer || s.Kind == sectionKindLegacyDynamic
}

// Title returns a container's title (options.title), falling back to the
// legacy name field.
func (s Section) Title() string {
	if len(s.Options) > 0 {
		var o containerOptions
		if err := json.Unmarshal(s.Options, &o); err == nil && o.Title != "" {
			return o.Title
		}
	}
	return s.Name
}

// MarshalJSON emits exactly the shape of the matching branch of homarr
// v2.12.0's section union, so the board.saveBoard input validates:
//
//	empty:     {id, kind, xOffset, yOffset}
//	container: {id, kind: "container", options, collapsed, layouts: [...]}
//
// Other kinds are passed through unchanged so homarr reports them instead of
// the controller silently rewriting data it does not understand.
func (s Section) MarshalJSON() ([]byte, error) {
	switch {
	case s.Kind == SectionKindEmpty:
		return json.Marshal(struct {
			ID      string `json:"id"`
			Kind    string `json:"kind"`
			XOffset int    `json:"xOffset"`
			YOffset int    `json:"yOffset"`
		}{s.ID, s.Kind, s.XOffset, s.YOffset})
	case s.IsContainer():
		collapsed := false
		if s.Collapsed != nil {
			collapsed = *s.Collapsed
		}
		options := s.Options
		if len(options) == 0 || string(options) == "null" {
			options = json.RawMessage(`{}`)
		}
		layouts := s.Layouts
		if layouts == nil {
			layouts = []SectionLayout{}
		}
		return json.Marshal(struct {
			ID        string          `json:"id"`
			Kind      string          `json:"kind"`
			Options   json.RawMessage `json:"options"`
			Collapsed bool            `json:"collapsed"`
			Layouts   []SectionLayout `json:"layouts"`
		}{s.ID, SectionKindContainer, options, collapsed, layouts})
	default:
		type alias Section
		return json.Marshal(alias(s))
	}
}

// NewContainerOptions returns complete container options for a controller-
// created category container, matching what homarr's own category migration
// writes (collapsible, label shown, "open all" enabled).
func NewContainerOptions(title string) json.RawMessage {
	if len(title) > containerTitleMaxLen {
		title = title[:containerTitleMaxLen]
	}
	b, _ := json.Marshal(map[string]any{
		"title":            title,
		"customCssClasses": []string{},
		"borderColor":      "",
		"showLabel":        true,
		"collapsible":      true,
		"showOpenAll":      true,
		"scrollable":       false,
	})
	return b
}
