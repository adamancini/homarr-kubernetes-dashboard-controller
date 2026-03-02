package source

import (
	"context"
	"fmt"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	networkingv1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IngressSource struct {
	client     client.Client
	prefix     string
	namespaces []string
}

func NewIngressSource(cl client.Client, prefix string, namespaces []string) *IngressSource {
	return &IngressSource{client: cl, prefix: prefix, namespaces: namespaces}
}

func (s *IngressSource) List(ctx context.Context) ([]DashboardEntry, error) {
	var entries []DashboardEntry
	namespaces := s.namespaces
	if len(namespaces) == 0 {
		namespaces = []string{""}
	}
	for _, ns := range namespaces {
		var list networkingv1.IngressList
		opts := []client.ListOption{}
		if ns != "" {
			opts = append(opts, client.InNamespace(ns))
		}
		if err := s.client.List(ctx, &list, opts...); err != nil {
			return nil, fmt.Errorf("list ingresses: %w", err)
		}
		for _, ing := range list.Items {
			if !IsEnabled(ing.Annotations, s.prefix) {
				continue
			}
			entry := ParseAnnotations(ing.Annotations, s.prefix)
			entry.ID = fmt.Sprintf("%s/Ingress/%s", ing.Namespace, ing.Name)

			if entry.Name == "" {
				entry.Name = cases.Title(language.English).String(ing.Name)
			}
			if entry.Group == "" {
				entry.Group = ing.Namespace
			}
			if entry.URL == "" {
				entry.URL = inferIngressURL(&ing)
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (s *IngressSource) SetupWithManager(mgr ctrl.Manager) error {
	return nil
}

func inferIngressURL(ing *networkingv1.Ingress) string {
	if len(ing.Spec.Rules) == 0 {
		return ""
	}
	host := ing.Spec.Rules[0].Host
	if host == "" {
		return ""
	}
	scheme := "http"
	for _, tls := range ing.Spec.TLS {
		for _, h := range tls.Hosts {
			if h == host {
				scheme = "https"
				break
			}
		}
	}
	return scheme + "://" + host
}
