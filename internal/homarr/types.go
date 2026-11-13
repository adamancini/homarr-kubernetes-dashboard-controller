package homarr

import "encoding/json"

type App struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	IconURL     string `json:"iconUrl"`
	Href        string `json:"href"`
	Description string `json:"description"`
	PingURL     string `json:"pingUrl"`
}

type AppCreate struct {
	Name        string `json:"name"`
	IconURL     string `json:"iconUrl"`
	Href        string `json:"href"`
	Description string `json:"description"`
	PingURL     string `json:"pingUrl"`
}

type AppUpdate struct {
	Name        string `json:"name"`
	IconURL     string `json:"iconUrl"`
	Href        string `json:"href"`
	Description string `json:"description"`
	PingURL     string `json:"pingUrl"`
}

type Board struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	IsPublic bool        `json:"isPublic"`
	Sections []Section   `json:"sections,omitempty"`
	Items    []BoardItem `json:"items,omitempty"`
	Layouts  []Layout    `json:"layouts,omitempty"`
}

type BoardCreate struct {
	Name        string `json:"name"`
	ColumnCount int    `json:"columnCount"`
	IsPublic    bool   `json:"isPublic"`
}

type BoardItem struct {
	ID              string          `json:"id"`
	Kind            string          `json:"kind"`
	Options         json.RawMessage `json:"options,omitempty"`
	Layouts         []ItemLayout    `json:"layouts"`
	IntegrationIDs  []string        `json:"integrationIds"`
	AdvancedOptions json.RawMessage `json:"advancedOptions,omitempty"`
}

type ItemLayout struct {
	LayoutID  string `json:"layoutId"`
	XOffset   int    `json:"xOffset"`
	YOffset   int    `json:"yOffset"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SectionID string `json:"sectionId"`
}

// Layout is one responsive layout of a board (homarr v2 has at least a
// "mobile" and a "base" layout). Items and container sections carry one
// placement per layout.
type Layout struct {
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	ColumnCount            int    `json:"columnCount"`
	Breakpoint             int    `json:"breakpoint"`
	LeftGutterColumnCount  *int   `json:"leftGutterColumnCount,omitempty"`
	RightGutterColumnCount *int   `json:"rightGutterColumnCount,omitempty"`
	Role                   string `json:"role,omitempty"`
}

// MainLaneColumnCount returns the number of columns of the board's main lane
// in this layout, i.e. the width available to the main root section. It
// mirrors getBoardLaneColumnCount(layout, "main") in homarr v2.12.0
// (packages/definitions/src/section.ts): gutters are ignored on mobile.
func (l Layout) MainLaneColumnCount() int {
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

type BoardSave struct {
	ID       string      `json:"id"`
	Sections []Section   `json:"sections"`
	Items    []BoardItem `json:"items"`
}

// MarshalJSON sends sections and items as arrays even when empty; homarr's
// boardSaveSchema rejects null for either.
func (b BoardSave) MarshalJSON() ([]byte, error) {
	type alias BoardSave
	a := alias(b)
	if a.Sections == nil {
		a.Sections = []Section{}
	}
	if a.Items == nil {
		a.Items = []BoardItem{}
	}
	return json.Marshal(a)
}

// integrationCreateResult is the result of homarr v2.12.0 integration.create:
// {integration: {id, name, kind, url}, appId} when the connection test passes,
// or {error: {...}} (still HTTP 200, nothing persisted) when it fails.
type integrationCreateResult struct {
	Integration *Integration    `json:"integration"`
	AppID       *string         `json:"appId"`
	Error       json.RawMessage `json:"error"`
}

type Integration struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	URL   string `json:"url"`
	Kind  string `json:"kind"`
	AppID string `json:"appId,omitempty"`
}

type IntegrationCreate struct {
	Name                        string              `json:"name"`
	URL                         string              `json:"url"`
	Kind                        string              `json:"kind"`
	Secrets                     []IntegrationSecret `json:"secrets"`
	AttemptSearchEngineCreation bool                `json:"attemptSearchEngineCreation"`
}

// MarshalJSON sends secrets as an array even when empty; homarr's
// integrationCreateSchema rejects null.
func (i IntegrationCreate) MarshalJSON() ([]byte, error) {
	type alias IntegrationCreate
	a := alias(i)
	if a.Secrets == nil {
		a.Secrets = []IntegrationSecret{}
	}
	return json.Marshal(a)
}

type IntegrationSecret struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}
