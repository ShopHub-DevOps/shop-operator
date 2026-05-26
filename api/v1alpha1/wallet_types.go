package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WalletPurpose describes what the Wallet is used for.
// +kubebuilder:validation:Enum=payments;payout
type WalletPurpose string

const (
	// WalletPurposePayments collects buyer payments for one or more Shops.
	WalletPurposePayments WalletPurpose = "payments"
	// WalletPurposePayout receives platform payouts to the ShopHub user.
	WalletPurposePayout WalletPurpose = "payout"
)

// WalletPhase reflects the high-level lifecycle position of a Wallet.
// +kubebuilder:validation:Enum=Pending;Ready;Failed;Terminating
type WalletPhase string

const (
	WalletPhasePending     WalletPhase = "Pending"
	WalletPhaseReady       WalletPhase = "Ready"
	WalletPhaseFailed      WalletPhase = "Failed"
	WalletPhaseTerminating WalletPhase = "Terminating"
)

// WalletSpec is the user-supplied configuration for a Wallet.
type WalletSpec struct {
	// DisplayName is the human-readable label shown in the ShopHub UI.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=80
	DisplayName string `json:"displayName"`

	// Address is the EVM wallet address. Stored as the user supplied it
	// (mixed-case checksum form is allowed); the operator does not normalise.
	// +kubebuilder:validation:Pattern=`^0x[a-fA-F0-9]{40}$`
	Address string `json:"address"`

	// ChainID is the EVM chain on which Address is valid.
	// Defaults to 11155111 (Sepolia testnet) if unset.
	// +kubebuilder:default=11155111
	// +kubebuilder:validation:Minimum=1
	// +optional
	ChainID int64 `json:"chainId,omitempty"`

	// Purpose declares how this Wallet is used. Defaults to "payments".
	// +kubebuilder:default=payments
	// +optional
	Purpose WalletPurpose `json:"purpose,omitempty"`

	// OwnerRef optionally identifies the ShopHub user that owns this Wallet.
	// Free-form to avoid coupling to a specific auth model (email, sub, etc.).
	// +kubebuilder:validation:MaxLength=253
	// +optional
	OwnerRef string `json:"ownerRef,omitempty"`
}

// WalletStatus is the reconciler-maintained observed state of a Wallet.
type WalletStatus struct {
	// Phase is the high-level lifecycle position.
	// +optional
	Phase WalletPhase `json:"phase,omitempty"`

	// Conditions follows the standard meta/v1 Condition pattern.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ObservedGeneration matches Wallet.metadata.generation when the status
	// was last computed, so clients can detect stale statuses.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=wlt,categories=shophub
// +kubebuilder:printcolumn:name="Address",type="string",JSONPath=".spec.address"
// +kubebuilder:printcolumn:name="Chain",type="integer",JSONPath=".spec.chainId"
// +kubebuilder:printcolumn:name="Purpose",type="string",JSONPath=".spec.purpose"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Wallet is a managed EVM wallet record in the ShopHub platform.
type Wallet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WalletSpec   `json:"spec,omitempty"`
	Status WalletStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WalletList is the list type for Wallet.
type WalletList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Wallet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Wallet{}, &WalletList{})
}
