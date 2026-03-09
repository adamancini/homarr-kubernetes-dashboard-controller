package source

import (
	"context"
	"fmt"
	"regexp"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var hostRegexp = regexp.MustCompile("Host\\(`([^`]+)`\\)")

type IngressRouteSource struct {
	client     client.Client
	prefix     string
	namespaces []string
}

func NewIngressRouteSource(cl client.Client, prefix string, namespaces []string) *IngressRouteSource {
	return &IngressRouteSource{client: cl, prefix: prefix, namespaces: namespaces}
}

func (s *IngressRouteSource) List(ctx context.Context) ([]DashboardEntry, error) {
	var entries []DashboardEntry
	namespaces := s.namespaces
	if len(namespaces) == 0 {
		namespaces = []string{""}
	}
	for _, ns := range namespaces {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group: "traefik.io", Version: "v1alpha1", Kind: "IngressRouteList",
		})
		opts := []client.ListOption{}
		if ns != "" {
			opts = append(opts, client.InNamespace(ns))
		}
		if err := s.client.List(ctx, list, opts...); err != nil {
			return nil, fmt.Errorf("list ingressroutes: %w", err)
		}
		for _, obj := range list.Items {
			annotations := obj.GetAnnotations()
			if !IsEnabled(annotations, s.prefix) {
				continue
			}
			entry := ParseAnnotations(annotations, s.prefix)
			entry.ID = fmt.Sprintf("%s/IngressRoute/%s", obj.GetNamespace(), obj.GetName())

			if entry.Name == "" {
				entry.Name = cases.Title(language.English).String(obj.GetName())
			}
			if entry.Category == "" {
				entry.Category = obj.GetNamespace()
			}
			if entry.URL == "" {
				entry.URL = inferIngressRouteURL(obj.Object)
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (s *IngressRouteSource) SetupWithManager(mgr ctrl.Manager) error {
	return nil
}

func inferIngressRouteURL(obj map[string]any) string {
	spec, _ := obj["spec"].(map[string]any)
	routes, _ := spec["routes"].([]any)
	for _, r := range routes {
		route, _ := r.(map[string]any)
		match, _ := route["match"].(string)
		host := ExtractHostFromMatch(match)
		if host != "" {
			return "https://" + host
		}
	}
	return ""
}

func ExtractHostFromMatch(match string) string {
	m := hostRegexp.FindStringSubmatch(match)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
