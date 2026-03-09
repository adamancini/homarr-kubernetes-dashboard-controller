package source

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
)

type DashboardEntry struct {
	// Identity
	ID string // "<namespace>/<kind>/<name>"

	// App fields
	Name        string
	URL         string
	IconURL     string
	Description string
	PingURL     string

	// Organization
	Category string
	Priority int

	// Integration
	IntegrationType      string
	IntegrationURL       string
	IntegrationSecret    string
	IntegrationSecretKey string

	// Widget
	Widget string
}

type Source interface {
	List(ctx context.Context) ([]DashboardEntry, error)
	SetupWithManager(mgr ctrl.Manager) error
}
