package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KubeSecretReader reads Kubernetes Secrets using a controller-runtime client.
type KubeSecretReader struct {
	client client.Client
}

func NewKubeSecretReader(c client.Client) *KubeSecretReader {
	return &KubeSecretReader{client: c}
}

func (r *KubeSecretReader) ReadSecret(ctx context.Context, namespace, name string) (map[string][]byte, error) {
	var secret corev1.Secret
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &secret); err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", namespace, name, err)
	}
	return secret.Data, nil
}
