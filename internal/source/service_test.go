package source_test

import (
	"context"
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/source"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestServiceSource_List(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sonarr",
			Namespace: "htpc",
			Annotations: map[string]string{
				"homarr.dev/enabled": "true",
				"homarr.dev/name":    "Sonarr",
				"homarr.dev/icon":    "sonarr",
				"homarr.dev/group":   "Media",
				"homarr.dev/url":     "https://sonarr.example.com",
			},
		},
	}

	// Service without annotation — should be skipped
	svcNoAnnotation := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "internal-svc",
			Namespace: "htpc",
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, svcNoAnnotation).Build()
	src := source.NewServiceSource(cl, "homarr.dev", nil)

	entries, err := src.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.ID != "htpc/Service/sonarr" {
		t.Errorf("ID = %q", e.ID)
	}
	if e.Name != "Sonarr" {
		t.Errorf("Name = %q", e.Name)
	}
	if e.URL != "https://sonarr.example.com" {
		t.Errorf("URL = %q, want https://sonarr.example.com", e.URL)
	}
}

func TestServiceSource_RequiresExplicitURL(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nourl",
			Namespace: "default",
			Annotations: map[string]string{
				"homarr.dev/enabled": "true",
				"homarr.dev/name":    "NoURL",
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()
	src := source.NewServiceSource(cl, "homarr.dev", nil)

	entries, err := src.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].URL != "" {
		t.Errorf("URL = %q, want empty (services require explicit URL)", entries[0].URL)
	}
}
