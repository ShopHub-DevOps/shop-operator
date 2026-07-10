package resources

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"
)

// BuildREDBDatabase composes a RedisEnterpriseDatabase for a Shop.
func BuildREDBDatabase(s *shophubv1alpha1.Shop) *unstructured.Unstructured {
	db := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "app.redislabs.com/v1alpha1",
			"kind":       "RedisEnterpriseDatabase",
			"metadata": map[string]interface{}{
				"name":      REDBDatabaseName(s),
				"namespace": s.Namespace,
				"labels":    ComponentLabels(s.Name, "database"),
			},
			"spec": map[string]interface{}{
				// Sensible default for a light tier
				"memoryLimit": "100MB",
			},
		},
	}
	return db
}

// REDBDatabaseName returns the standard name for the Redis resource.
func REDBDatabaseName(s *shophubv1alpha1.Shop) string {
	return s.Name + "-redis"
}

// REDBSecretName returns the name of the REDB-generated secret.
// REDB automatically generates a secret with the same name as the database.
func REDBSecretName(s *shophubv1alpha1.Shop) string {
	return "redb-" + REDBDatabaseName(s)
}
