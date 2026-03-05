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

type Section struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Name      string          `json:"name,omitempty"`
	XOffset   int             `json:"xOffset"`
	YOffset   int             `json:"yOffset"`
	Collapsed *bool           `json:"collapsed,omitempty"`
	Options   json.RawMessage `json:"options,omitempty"`
	Layouts   json.RawMessage `json:"layouts,omitempty"`
}

type BoardItem struct {
	ID              string          `json:"id"`
	Kind            string          `json:"kind"`
	XOffset         int             `json:"xOffset"`
	YOffset         int             `json:"yOffset"`
	Width           int             `json:"width"`
	Height          int             `json:"height"`
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

type Layout struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ColumnCount int    `json:"columnCount"`
	Breakpoint  int    `json:"breakpoint"`
}

type BoardSave struct {
	ID       string      `json:"id"`
	Sections []Section   `json:"sections"`
	Items    []BoardItem `json:"items"`
}

type Integration struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	URL   string `json:"url"`
	Kind  string `json:"kind"`
	AppID string `json:"appId,omitempty"`
}

type IntegrationCreate struct {
	Name    string              `json:"name"`
	URL     string              `json:"url"`
	Kind    string              `json:"kind"`
	Secrets []IntegrationSecret `json:"secrets"`
}

type IntegrationSecret struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}
