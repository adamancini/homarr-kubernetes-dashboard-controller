package source

import (
	"context"
	"fmt"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type HTTPRouteSource struct {
	client     client.Client
	prefix     string
	namespaces []string
}

func NewHTTPRouteSource(cl client.Client, prefix string, namespaces []string) *HTTPRouteSource {
	return &HTTPRouteSource{client: cl, prefix: prefix, namespaces: namespaces}
}

func (s *HTTPRouteSource) List(ctx context.Context) ([]DashboardEntry, error) {
	var entries []DashboardEntry
	namespaces := s.namespaces
	if len(namespaces) == 0 {
		namespaces = []string{""}
	}
	for _, ns := range namespaces {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRouteList",
		})
		opts := []client.ListOption{}
		if ns != "" {
			opts = append(opts, client.InNamespace(ns))
		}
		if err := s.client.List(ctx, list, opts...); err != nil {
			return nil, fmt.Errorf("list httproutes: %w", err)
		}
		for _, obj := range list.Items {
			annotations := obj.GetAnnotations()
			if !IsEnabled(annotations, s.prefix) {
				continue
			}
			entry := ParseAnnotations(annotations, s.prefix)
			entry.ID = fmt.Sprintf("%s/HTTPRoute/%s", obj.GetNamespace(), obj.GetName())

			if entry.Name == "" {
				entry.Name = cases.Title(language.English).String(obj.GetName())
			}
			if entry.Group == "" {
				entry.Group = obj.GetNamespace()
			}
			if entry.URL == "" {
				entry.URL = inferHTTPRouteURL(obj.Object)
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (s *HTTPRouteSource) SetupWithManager(mgr ctrl.Manager) error {
	return nil
}

func inferHTTPRouteURL(obj map[string]any) string {
	spec, _ := obj["spec"].(map[string]any)
	hostnames, _ := spec["hostnames"].([]any)
	for _, h := range hostnames {
		hostname, _ := h.(string)
		if hostname != "" {
			return "https://" + hostname
		}
	}
	return ""
}
