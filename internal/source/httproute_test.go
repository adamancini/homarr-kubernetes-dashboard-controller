package source_test

import (
	"context"
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/source"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHTTPRouteSource_List(t *testing.T) {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRouteList"},
		&unstructured.UnstructuredList{},
	)

	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute",
	})
	route.SetName("grafana")
	route.SetNamespace("monitoring")
	route.SetAnnotations(map[string]string{
		"homarr.dev/enabled": "true",
		"homarr.dev/name":    "Grafana",
		"homarr.dev/icon":    "grafana",
		"homarr.dev/category": "Infra",
	})
	route.Object["spec"] = map[string]any{
		"hostnames": []any{"grafana.example.com"},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(route).Build()
	src := source.NewHTTPRouteSource(cl, "homarr.dev", nil)

	entries, err := src.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].URL != "https://grafana.example.com" {
		t.Errorf("URL = %q, want https://grafana.example.com", entries[0].URL)
	}
	if entries[0].ID != "monitoring/HTTPRoute/grafana" {
		t.Errorf("ID = %q", entries[0].ID)
	}
}

func TestHTTPRouteSource_NoHostname(t *testing.T) {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRouteList"},
		&unstructured.UnstructuredList{},
	)

	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute",
	})
	route.SetName("nohost")
	route.SetNamespace("default")
	route.SetAnnotations(map[string]string{
		"homarr.dev/enabled": "true",
	})
	route.Object["spec"] = map[string]any{}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(route).Build()
	src := source.NewHTTPRouteSource(cl, "homarr.dev", nil)

	entries, err := src.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].URL != "" {
		t.Errorf("URL = %q, want empty", entries[0].URL)
	}
}
