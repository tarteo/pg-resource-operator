package v1

type DatabasePrivilege struct {
	Role RoleSource `json:"role"`
	// +kubebuilder:default:=false
	Connect bool `json:"connect"`
	// +kubebuilder:default:=false
	Create bool `json:"create"`
	// +kubebuilder:default:=false
	Temporary bool `json:"temporary"`
}
