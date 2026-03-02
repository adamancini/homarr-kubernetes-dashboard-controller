package state_test

import (
	"context"
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/state"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestStateManager_RoundTrip(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	mgr := state.NewManager(cl, "homarr", "homarr-dashboard-controller-state")

	ctx := context.Background()

	// Load from nonexistent ConfigMap should succeed (empty state)
	if err := mgr.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Set some mappings
	mgr.SetApp("homarr-id-1", "htpc/Ingress/sonarr")
	mgr.SetApp("homarr-id-2", "htpc/Ingress/radarr")
	mgr.SetIntegration("intg-1", "htpc/Ingress/sonarr")

	// Save
	if err := mgr.Save(ctx); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load into fresh manager
	mgr2 := state.NewManager(cl, "homarr", "homarr-dashboard-controller-state")
	if err := mgr2.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if mgr2.GetAppSource("homarr-id-1") != "htpc/Ingress/sonarr" {
		t.Errorf("unexpected source for homarr-id-1: %q", mgr2.GetAppSource("homarr-id-1"))
	}
	if mgr2.GetIntegrationSource("intg-1") != "htpc/Ingress/sonarr" {
		t.Errorf("unexpected source for intg-1")
	}

	// Delete and verify
	mgr2.RemoveApp("homarr-id-1")
	if mgr2.GetAppSource("homarr-id-1") != "" {
		t.Error("expected empty after remove")
	}

	// ManagedAppIDs
	ids := mgr2.ManagedAppIDs()
	if len(ids) != 1 || ids[0] != "homarr-id-2" {
		t.Errorf("ManagedAppIDs = %v", ids)
	}
}

func TestStateManager_FindAppBySource(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	mgr := state.NewManager(cl, "homarr", "test-state")
	mgr.SetApp("app-1", "htpc/Ingress/sonarr")

	id, ok := mgr.FindAppBySource("htpc/Ingress/sonarr")
	if !ok || id != "app-1" {
		t.Errorf("FindAppBySource = %q, %v", id, ok)
	}

	_, ok = mgr.FindAppBySource("htpc/Ingress/radarr")
	if ok {
		t.Error("expected not found")
	}
}
