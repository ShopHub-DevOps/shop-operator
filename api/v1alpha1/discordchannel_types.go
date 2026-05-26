package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DiscordSeverity is the minimum alert severity routed to a DiscordChannel.
// +kubebuilder:validation:Enum=info;warning;critical
type DiscordSeverity string

const (
	DiscordSeverityInfo     DiscordSeverity = "info"
	DiscordSeverityWarning  DiscordSeverity = "warning"
	DiscordSeverityCritical DiscordSeverity = "critical"
)

// DiscordChannelPhase reflects the high-level lifecycle position of a DiscordChannel.
// +kubebuilder:validation:Enum=Pending;Ready;Failed;Terminating
type DiscordChannelPhase string

const (
	DiscordChannelPhasePending     DiscordChannelPhase = "Pending"
	DiscordChannelPhaseReady       DiscordChannelPhase = "Ready"
	DiscordChannelPhaseFailed      DiscordChannelPhase = "Failed"
	DiscordChannelPhaseTerminating DiscordChannelPhase = "Terminating"
)

// DiscordWebhookSecretRef points at a Secret in the same namespace that holds
// the Discord webhook URL under the key `url`. The URL itself is never stored
// in the CR to keep webhooks out of plain etcd manifests.
type DiscordWebhookSecretRef struct {
	// Name of the Secret in the same namespace as the DiscordChannel.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// Key inside the Secret whose value is the webhook URL.
	// Defaults to "url" if unset.
	// +kubebuilder:default=url
	// +optional
	Key string `json:"key,omitempty"`
}

// DiscordChannelSpec is the user-supplied configuration for a DiscordChannel.
type DiscordChannelSpec struct {
	// ChannelName is the human-readable label for this channel, shown in the
	// ShopHub UI and used as the default username when posting to Discord.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=80
	ChannelName string `json:"channelName"`

	// WebhookSecretRef points at the Secret carrying the Discord webhook URL.
	WebhookSecretRef DiscordWebhookSecretRef `json:"webhookSecretRef"`

	// ShopRef optionally scopes alerts to a single Shop in the same namespace.
	// When empty, the channel receives alerts for every Shop in the namespace.
	// +optional
	ShopRef string `json:"shopRef,omitempty"`

	// MinSeverity filters which alerts are routed here. Alerts strictly below
	// this severity are dropped. Defaults to "warning".
	// +kubebuilder:default=warning
	// +optional
	MinSeverity DiscordSeverity `json:"minSeverity,omitempty"`
}

// DiscordChannelStatus is the reconciler-maintained observed state of a DiscordChannel.
type DiscordChannelStatus struct {
	// Phase is the high-level lifecycle position.
	// +optional
	Phase DiscordChannelPhase `json:"phase,omitempty"`

	// LastValidatedAt records the time the referenced webhook Secret was last
	// verified to exist with the expected key.
	// +optional
	LastValidatedAt *metav1.Time `json:"lastValidatedAt,omitempty"`

	// Conditions follows the standard meta/v1 Condition pattern.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ObservedGeneration matches DiscordChannel.metadata.generation when the
	// status was last computed, so clients can detect stale statuses.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dch,categories=shophub
// +kubebuilder:printcolumn:name="Channel",type="string",JSONPath=".spec.channelName"
// +kubebuilder:printcolumn:name="Shop",type="string",JSONPath=".spec.shopRef"
// +kubebuilder:printcolumn:name="MinSeverity",type="string",JSONPath=".spec.minSeverity"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DiscordChannel registers a Discord webhook destination for alerts emitted
// by the ShopHub platform.
type DiscordChannel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DiscordChannelSpec   `json:"spec,omitempty"`
	Status DiscordChannelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DiscordChannelList is the list type for DiscordChannel.
type DiscordChannelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiscordChannel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DiscordChannel{}, &DiscordChannelList{})
}
