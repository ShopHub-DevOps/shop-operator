package resources

import shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"

// Naming convention for child resources of a Shop:
//
//	<shop>-be       backend Deployment + Service
//	<shop>-fe       frontend Deployment + Service
//	<shop>-config   ConfigMap with public env (wallet, chain, db tier)
//	<shop>-secret   Secret with sensitive env (JWT, DB password)
//	<shop>          Ingress

func BackendName(s *shophubv1alpha1.Shop) string  { return s.Name + "-be" }
func FrontendName(s *shophubv1alpha1.Shop) string { return s.Name + "-fe" }
func ConfigMapName(s *shophubv1alpha1.Shop) string {
	return s.Name + "-config"
}
func SecretName(s *shophubv1alpha1.Shop) string  { return s.Name + "-secret" }
func IngressName(s *shophubv1alpha1.Shop) string { return s.Name }

// Replicas returns the deployment replica count derived from the availability tier.
func Replicas(s *shophubv1alpha1.Shop) int32 {
	if s.Spec.Availability == shophubv1alpha1.AvailabilityHigh {
		return 3
	}
	return 2
}

// BackendImage returns the backend container image to use, honoring spec.images.backend.
func BackendImage(s *shophubv1alpha1.Shop) string {
	if s.Spec.Images.Backend != "" {
		return s.Spec.Images.Backend
	}
	return DefaultBackendImage
}

// FrontendImage returns the frontend container image to use, honoring spec.images.frontend.
func FrontendImage(s *shophubv1alpha1.Shop) string {
	if s.Spec.Images.Frontend != "" {
		return s.Spec.Images.Frontend
	}
	return DefaultFrontendImage
}
