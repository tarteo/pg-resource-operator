package v1

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:validation:XValidation:rule="has(self.name) != has(self.secretKeyRef)",message="name or secretKeyRef must be set"
type RoleSource struct {
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`

	// +kubebuilder:validation:Optional
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeyRef,omitempty"`
}

func (r *RoleSource) GetName(ctx context.Context, c client.Client, ownerNamespace string) (string, error) {
	if r.Name != "" {
		return r.Name, nil
	}

	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: ownerNamespace, Name: r.SecretKeyRef.Name}, &secret); err != nil {
		return "", err
	}

	role, found := secret.Data[r.SecretKeyRef.Key]
	if !found {
		return "", fmt.Errorf("unable to find key %q in secret %q", r.SecretKeyRef.Key, r.SecretKeyRef.Name)
	}

	if len(role) == 0 {
		return "", fmt.Errorf("role value in secret %q key %q is empty", r.SecretKeyRef.Name, r.SecretKeyRef.Key)
	}

	return string(role), nil
}
