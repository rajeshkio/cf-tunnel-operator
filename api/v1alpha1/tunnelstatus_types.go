package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type TunnelStatus struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TunnelStatusSpec   `json:"spec,omitempty"`
	Status            TunnelStatusStatus `json:"status,omitempty"`
}

// +kubebuilder:object:generate=true
type TunnelStatusStatus struct {
	Hostname       string      `json:"hostname,omitempty"`
	BackendService string      `json:"backendService"`
	LastSyncTime   metav1.Time `json:"lastSyncTime"`
	SyncStatus     string      `json:"syncStatus"`
	Scheme         string      `json:"scheme"`
	NoTLSVerify    bool        `json:"notlsverify"`
	Message        string      `json:"message"`
}

type TunnelStatusSpec struct {
	HTTPRouteNamespace string `json:"httpRouteNamespace"`
	HTTPRouteName      string `json:"httpRouteName"`
}

// +kubebuilder:object:root=true
type TunnelStatusList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TunnelStatus `json:"tunnelStatus"`
}

func init() {
	SchemeBuilder.Register(&TunnelStatus{}, &TunnelStatusList{})
}
