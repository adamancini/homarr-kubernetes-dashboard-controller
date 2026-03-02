package state

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InMemoryState is the in-memory ownership map used by the reconciler.
// Manager wraps this with ConfigMap persistence.
type InMemoryState struct {
	apps         map[string]string
	integrations map[string]string
}

func NewInMemoryState() *InMemoryState {
	return &InMemoryState{
		apps:         make(map[string]string),
		integrations: make(map[string]string),
	}
}

func (s *InMemoryState) SetApp(homarrID, sourceID string) { s.apps[homarrID] = sourceID }
func (s *InMemoryState) SetIntegration(homarrID, sourceID string) {
	s.integrations[homarrID] = sourceID
}
func (s *InMemoryState) RemoveApp(homarrID string)                   { delete(s.apps, homarrID) }
func (s *InMemoryState) RemoveIntegration(homarrID string)           { delete(s.integrations, homarrID) }
func (s *InMemoryState) GetAppSource(homarrID string) string         { return s.apps[homarrID] }
func (s *InMemoryState) GetIntegrationSource(homarrID string) string { return s.integrations[homarrID] }

func (s *InMemoryState) FindAppBySource(sourceID string) (string, bool) {
	for homarrID, src := range s.apps {
		if src == sourceID {
			return homarrID, true
		}
	}
	return "", false
}

func (s *InMemoryState) FindIntegrationBySource(sourceID string) (string, bool) {
	for homarrID, src := range s.integrations {
		if src == sourceID {
			return homarrID, true
		}
	}
	return "", false
}

func (s *InMemoryState) ManagedAppIDs() []string {
	ids := make([]string, 0, len(s.apps))
	for id := range s.apps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (s *InMemoryState) ManagedIntegrationIDs() []string {
	ids := make([]string, 0, len(s.integrations))
	for id := range s.integrations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Manager wraps InMemoryState with ConfigMap-backed persistence.
type Manager struct {
	InMemoryState
	client    client.Client
	namespace string
	name      string
}

func NewManager(cl client.Client, namespace, name string) *Manager {
	return &Manager{
		InMemoryState: InMemoryState{
			apps:         make(map[string]string),
			integrations: make(map[string]string),
		},
		client:    cl,
		namespace: namespace,
		name:      name,
	}
}

func (m *Manager) Load(ctx context.Context) error {
	var cm corev1.ConfigMap
	err := m.client.Get(ctx, types.NamespacedName{Namespace: m.namespace, Name: m.name}, &cm)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get state configmap: %w", err)
	}

	if data, ok := cm.Data["managed-apps"]; ok {
		if err := json.Unmarshal([]byte(data), &m.apps); err != nil {
			return fmt.Errorf("unmarshal managed-apps: %w", err)
		}
	}
	if data, ok := cm.Data["managed-integrations"]; ok {
		if err := json.Unmarshal([]byte(data), &m.integrations); err != nil {
			return fmt.Errorf("unmarshal managed-integrations: %w", err)
		}
	}
	return nil
}

func (m *Manager) Save(ctx context.Context) error {
	appsJSON, err := json.Marshal(m.apps)
	if err != nil {
		return err
	}
	intgJSON, err := json.Marshal(m.integrations)
	if err != nil {
		return err
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.name,
			Namespace: m.namespace,
		},
		Data: map[string]string{
			"managed-apps":         string(appsJSON),
			"managed-integrations": string(intgJSON),
		},
	}

	var existing corev1.ConfigMap
	err = m.client.Get(ctx, types.NamespacedName{Namespace: m.namespace, Name: m.name}, &existing)
	if errors.IsNotFound(err) {
		return m.client.Create(ctx, cm)
	}
	if err != nil {
		return err
	}
	existing.Data = cm.Data
	return m.client.Update(ctx, &existing)
}
