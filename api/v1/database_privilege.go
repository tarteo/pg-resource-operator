package v1

import (
	corev1 "k8s.io/api/core/v1"
)

// +kubebuilder:validation:XValidation:rule="has(self.name) != has(self.secretKeyRef)",message="name or secretKeyRef must be set"
type DatabasePrivilegeRole struct {
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`

	// +kubebuilder:validation:Optional
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeyRef,omitempty"`
}

type DatabasePrivilege struct {
	Role DatabasePrivilegeRole `json:"role"`
	// +kubebuilder:default:=false
	Connect bool `json:"connect"`
	// +kubebuilder:default:=false
	Create bool `json:"create"`
	// +kubebuilder:default:=false
	Temporary bool `json:"temporary"`
}
