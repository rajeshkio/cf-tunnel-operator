// +groupName=cf-tunnel-operator.rajesh-kumar.in
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (

	// SchemeGroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "cf-tunnel-operator.rajesh-kumar.in", Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme is required by pkg/client/...
	AddToScheme = SchemeBuilder.AddToScheme
)
