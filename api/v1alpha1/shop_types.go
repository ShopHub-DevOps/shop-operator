package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AvailabilityTier controls the per-Deployment replica count.
// +kubebuilder:validation:Enum=standard;high
type AvailabilityTier string

const (
	// AvailabilityStandard runs 2 replicas per Shop Deployment.
	AvailabilityStandard AvailabilityTier = "standard"
	// AvailabilityHigh runs 3 replicas per Shop Deployment.
	AvailabilityHigh AvailabilityTier = "high"
)

// DatabaseTier selects which database backend the Shop reconciler should provision.
// +kubebuilder:validation:Enum=standard;light
type DatabaseTier string

const (
	// DatabaseStandard provisions PostgreSQL via CNPG.
	DatabaseStandard DatabaseTier = "standard"
	// DatabaseLight provisions Redis via REDB.
	DatabaseLight DatabaseTier = "light"
)

// ShopPhase reflects the high-level lifecycle position of a Shop.
// +kubebuilder:validation:Enum=Pending;Provisioning;Ready;Failed;Terminating
type ShopPhase string

const (
	ShopPhasePending      ShopPhase = "Pending"
	ShopPhaseProvisioning ShopPhase = "Provisioning"
	ShopPhaseReady        ShopPhase = "Ready"
	ShopPhaseFailed       ShopPhase = "Failed"
	ShopPhaseTerminating  ShopPhase = "Terminating"
)

// ShopImages allows overriding the default container images per Shop.
// All fields are optional; empty values fall back to chart-level defaults.
type ShopImages struct {
	// +optional
	Backend string `json:"backend,omitempty"`
	// +optional
	Frontend string `json:"frontend,omitempty"`
}

// ShopSpec is the user-supplied configuration for a Shop instance.
type ShopSpec struct {
	// DisplayName is the human-readable name shown in the ShopHub panel.
	// May contain spaces and Unicode; does not need to match metadata.name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=80
	DisplayName string `json:"displayName"`

	// Host is the DNS name under which the Shop is exposed via Ingress.
	// Must be DNS-1123 compliant (lowercase, hyphen, dot).
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)+$`
	// +kubebuilder:validation:MaxLength=253
	Host string `json:"host"`

	// Availability controls Deployment replica count (standard=2, high=3).
	Availability AvailabilityTier `json:"availability"`

	// DatabaseTier selects PostgreSQL via CNPG (standard) or Redis via REDB (light).
	DatabaseTier DatabaseTier `json:"databaseTier"`

	// WalletAddress receives crypto payments for this Shop's orders.
	// Lowercase hex EVM address.
	// +kubebuilder:validation:Pattern=`^0x[a-fA-F0-9]{40}$`
	WalletAddress string `json:"walletAddress"`

	// ChainID is the EVM chain on which WalletAddress is valid.
	// Defaults to 11155111 (Sepolia testnet) if unset.
	// +kubebuilder:default=11155111
	// +kubebuilder:validation:Minimum=1
	// +optional
	ChainID int64 `json:"chainId,omitempty"`

	// Images optionally pins container image tags for backend and frontend.
	// +optional
	Images ShopImages `json:"images,omitempty"`
}

// ShopStatus is the reconciler-maintained observed state of a Shop.
type ShopStatus struct {
	// Phase is the high-level lifecycle position.
	// +optional
	Phase ShopPhase `json:"phase,omitempty"`

	// URL is the externally reachable URL once the Shop is Ready.
	// +optional
	URL string `json:"url,omitempty"`

	// Conditions follows the standard meta/v1 Condition pattern.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ObservedGeneration matches Shop.metadata.generation when the status
	// was last computed, so clients can detect stale statuses.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=shp,categories=shophub
// +kubebuilder:printcolumn:name="Host",type="string",JSONPath=".spec.host"
// +kubebuilder:printcolumn:name="Availability",type="string",JSONPath=".spec.availability"
// +kubebuilder:printcolumn:name="DB",type="string",JSONPath=".spec.databaseTier"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Shop is a tenant store managed by the shop-operator.
type Shop struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ShopSpec   `json:"spec,omitempty"`
	Status ShopStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ShopList is the list type for Shop.
type ShopList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Shop `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Shop{}, &ShopList{})
}
