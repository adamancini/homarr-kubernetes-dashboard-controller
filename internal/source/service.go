package source

import (
	"context"
	"fmt"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceSource struct {
	client     client.Client
	prefix     string
	namespaces []string
}

func NewServiceSource(cl client.Client, prefix string, namespaces []string) *ServiceSource {
	return &ServiceSource{client: cl, prefix: prefix, namespaces: namespaces}
}

func (s *ServiceSource) List(ctx context.Context) ([]DashboardEntry, error) {
	var entries []DashboardEntry
	namespaces := s.namespaces
	if len(namespaces) == 0 {
		namespaces = []string{""}
	}
	for _, ns := range namespaces {
		var list corev1.ServiceList
		opts := []client.ListOption{}
		if ns != "" {
			opts = append(opts, client.InNamespace(ns))
		}
		if err := s.client.List(ctx, &list, opts...); err != nil {
			return nil, fmt.Errorf("list services: %w", err)
		}
		for _, svc := range list.Items {
			if !IsEnabled(svc.Annotations, s.prefix) {
				continue
			}
			entry := ParseAnnotations(svc.Annotations, s.prefix)
			entry.ID = fmt.Sprintf("%s/Service/%s", svc.Namespace, svc.Name)

			if entry.Name == "" {
				entry.Name = cases.Title(language.English).String(svc.Name)
			}
			if entry.Group == "" {
				entry.Group = svc.Namespace
			}
			// Services have no host to infer — URL must come from annotation
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (s *ServiceSource) SetupWithManager(mgr ctrl.Manager) error {
	return nil
}
