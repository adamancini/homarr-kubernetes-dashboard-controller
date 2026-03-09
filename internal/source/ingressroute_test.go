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

func TestIngressRouteSource_List(t *testing.T) {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "traefik.io", Version: "v1alpha1", Kind: "IngressRouteList"},
		&unstructured.UnstructuredList{},
	)

	ir := &unstructured.Unstructured{}
	ir.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "traefik.io", Version: "v1alpha1", Kind: "IngressRoute",
	})
	ir.SetName("grafana")
	ir.SetNamespace("monitoring")
	ir.SetAnnotations(map[string]string{
		"homarr.dev/enabled": "true",
		"homarr.dev/name":    "Grafana",
		"homarr.dev/icon":    "grafana",
		"homarr.dev/category": "Infra",
	})
	ir.Object["spec"] = map[string]any{
		"routes": []any{
			map[string]any{"match": "Host(`grafana.example.com`)"},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ir).Build()
	src := source.NewIngressRouteSource(cl, "homarr.dev", nil)

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
	if entries[0].ID != "monitoring/IngressRoute/grafana" {
		t.Errorf("ID = %q", entries[0].ID)
	}
}

func TestExtractHostFromMatch(t *testing.T) {
	tests := []struct {
		match string
		want  string
	}{
		{"Host(`grafana.example.com`)", "grafana.example.com"},
		{"Host(`foo.bar.com`) && PathPrefix(`/api`)", "foo.bar.com"},
		{"PathPrefix(`/health`)", ""},
	}
	for _, tt := range tests {
		got := source.ExtractHostFromMatch(tt.match)
		if got != tt.want {
			t.Errorf("ExtractHostFromMatch(%q) = %q, want %q", tt.match, got, tt.want)
		}
	}
}
