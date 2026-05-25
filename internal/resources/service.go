package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"
)

// BuildBackendService exposes the backend Pods on port 80 inside the cluster.
func BuildBackendService(s *shophubv1alpha1.Shop) *corev1.Service {
	return buildService(s, BackendName(s), ComponentBackend)
}

// BuildFrontendService exposes the frontend Pods on port 80 inside the cluster.
func BuildFrontendService(s *shophubv1alpha1.Shop) *corev1.Service {
	return buildService(s, FrontendName(s), ComponentFrontend)
}

func buildService(s *shophubv1alpha1.Shop, name, component string) *corev1.Service {
	labels := ComponentLabels(s.Name, component)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromString("http"),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}
