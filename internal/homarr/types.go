package homarr

type App struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	IconURL     string `json:"iconUrl"`
	Href        string `json:"href"`
	Description string `json:"description,omitempty"`
	PingURL     string `json:"pingUrl,omitempty"`
}

type AppCreate struct {
	Name        string `json:"name"`
	IconURL     string `json:"iconUrl"`
	Href        string `json:"href"`
	Description string `json:"description,omitempty"`
	PingURL     string `json:"pingUrl,omitempty"`
}

type AppUpdate struct {
	Name        string `json:"name"`
	IconURL     string `json:"iconUrl"`
	Href        string `json:"href"`
	Description string `json:"description,omitempty"`
	PingURL     string `json:"pingUrl,omitempty"`
}
