package source_test

import (
	"context"
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/source"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIngressSource_List(t *testing.T) {
	scheme := runtime.NewScheme()
	networkingv1.AddToScheme(scheme)

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sonarr",
			Namespace: "htpc",
			Annotations: map[string]string{
				"homarr.dev/enabled": "true",
				"homarr.dev/name":    "Sonarr",
				"homarr.dev/icon":    "sonarr",
				"homarr.dev/category": "Services",
			},
		},
		Spec: networkingv1.IngressSpec{
			TLS: []networkingv1.IngressTLS{{Hosts: []string{"sonarr.example.com"}}},
			Rules: []networkingv1.IngressRule{
				{Host: "sonarr.example.com"},
			},
		},
	}

	// Ingress without annotation — should be skipped
	ingressNoAnnotation := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "internal-svc",
			Namespace: "htpc",
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{Host: "internal.example.com"}},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ingress, ingressNoAnnotation).Build()
	src := source.NewIngressSource(cl, "homarr.dev", nil)

	entries, err := src.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.ID != "htpc/Ingress/sonarr" {
		t.Errorf("ID = %q", e.ID)
	}
	if e.Name != "Sonarr" {
		t.Errorf("Name = %q", e.Name)
	}
	if e.URL != "https://sonarr.example.com" {
		t.Errorf("URL = %q, want https://sonarr.example.com", e.URL)
	}
	if e.Category != "Services" {
		t.Errorf("Category = %q", e.Category)
	}
}

func TestIngressSource_URLInference(t *testing.T) {
	scheme := runtime.NewScheme()
	networkingv1.AddToScheme(scheme)

	// No TLS = http
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "app",
			Namespace:   "default",
			Annotations: map[string]string{"homarr.dev/enabled": "true"},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{Host: "app.example.com"}},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ingress).Build()
	src := source.NewIngressSource(cl, "homarr.dev", nil)

	entries, _ := src.List(context.Background())
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].URL != "http://app.example.com" {
		t.Errorf("URL = %q, want http://app.example.com", entries[0].URL)
	}
}

func TestIngressSource_NamespaceFilter(t *testing.T) {
	scheme := runtime.NewScheme()
	networkingv1.AddToScheme(scheme)

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "app",
			Namespace:   "other",
			Annotations: map[string]string{"homarr.dev/enabled": "true"},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{Host: "app.example.com"}},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ingress).Build()
	// Only watch "htpc" namespace
	src := source.NewIngressSource(cl, "homarr.dev", []string{"htpc"})

	entries, _ := src.List(context.Background())
	if len(entries) != 0 {
		t.Errorf("expected 0 entries (filtered by namespace), got %d", len(entries))
	}
}
