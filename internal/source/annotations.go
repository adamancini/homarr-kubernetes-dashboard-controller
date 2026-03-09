package source

import (
	"strconv"
)

func IsEnabled(annotations map[string]string, prefix string) bool {
	return annotations[prefix+"/enabled"] == "true"
}

func ParseAnnotations(annotations map[string]string, prefix string) DashboardEntry {
	priority, _ := strconv.Atoi(annotations[prefix+"/priority"])

	// Read category annotation, falling back to deprecated group annotation
	category := annotations[prefix+"/category"]
	if category == "" {
		category = annotations[prefix+"/group"]
	}

	return DashboardEntry{
		Name:                 annotations[prefix+"/name"],
		URL:                  annotations[prefix+"/url"],
		IconURL:              annotations[prefix+"/icon"],
		Description:          annotations[prefix+"/description"],
		PingURL:              annotations[prefix+"/ping-url"],
		Category:             category,
		Priority:             priority,
		IntegrationType:      annotations[prefix+"/integration-type"],
		IntegrationURL:       annotations[prefix+"/integration-url"],
		IntegrationSecret:    annotations[prefix+"/integration-secret"],
		IntegrationSecretKey: annotations[prefix+"/integration-secret-key"],
		Widget:               annotations[prefix+"/widget"],
	}
}
