package resources

import (
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"
)

// BuildIngress wires `spec.host` to the frontend by default and to the backend at /api.
// The frontend serves the Shop UI at the root; the backend exposes its REST API under /api.
func BuildIngress(s *shophubv1alpha1.Shop) *networkingv1.Ingress {
	pathPrefix := networkingv1.PathTypePrefix
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      IngressName(s),
			Namespace: s.Namespace,
			Labels:    CommonLabels(s.Name),
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: s.Spec.Host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path:     "/api",
								PathType: &pathPrefix,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: BackendName(s),
										Port: networkingv1.ServiceBackendPort{Number: 80},
									},
								},
							},
							{
								Path:     "/",
								PathType: &pathPrefix,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: FrontendName(s),
										Port: networkingv1.ServiceBackendPort{Number: 80},
									},
								},
							},
						},
					},
				},
			}},
		},
	}
}
