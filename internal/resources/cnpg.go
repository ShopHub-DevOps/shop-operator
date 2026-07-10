package resources

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"
)

// BuildCNPGCluster composes a PostgreSQL Cluster for a Shop as unstructured resource.
func BuildCNPGCluster(s *shophubv1alpha1.Shop) *unstructured.Unstructured {
	instances := 3
	if s.Spec.Availability == shophubv1alpha1.AvailabilityStandard {
		instances = 2
	}

	cluster := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Cluster",
			"metadata": map[string]any{
				"name":      CNPGClusterName(s),
				"namespace": s.Namespace,
				"labels":    ComponentLabels(s.Name, "database"),
			},
			"spec": map[string]any{
				"instances": instances,
				"bootstrap": map[string]any{
					"initdb": map[string]any{
						"database": "shop",
						"owner":    "shop",
					},
				},
				"storage": map[string]any{
					"size": "10Gi",
				},
			},
		},
	}
	return cluster
}

func CNPGClusterName(s *shophubv1alpha1.Shop) string {
	return s.Name + "-db"
}

// CNPGSecretName returns the name of the CNPG-generated secret for a Shop.
// CNPG creates secrets with suffix "-app" for application credentials.
func CNPGSecretName(s *shophubv1alpha1.Shop) string {
	return CNPGClusterName(s) + "-app"
}
