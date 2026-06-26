package resources

import (
	"crypto/rand"
	"encoding/base64"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"
)

// BuildConfigMap exposes non-sensitive Shop configuration to its backend and
// frontend containers via envFrom. Sensitive values live in BuildSecret.
func BuildConfigMap(s *shophubv1alpha1.Shop) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName(s),
			Namespace: s.Namespace,
			Labels:    CommonLabels(s.Name),
		},
		Data: map[string]string{
			"SHOP_DISPLAY_NAME": s.Spec.DisplayName,
			"SHOP_HOST":         s.Spec.Host,
			"SHOP_DB_TIER":      string(s.Spec.DatabaseTier),
			"WALLET_ADDRESS":    s.Spec.WalletAddress,
			"CHAIN_ID":          strconv.FormatInt(s.Spec.ChainID, 10),

			"NEXT_PUBLIC_API_URL": "/api",
		},
	}
}

// BuildSecret holds sensitive values (JWT signing key, database password).
func BuildSecret(s *shophubv1alpha1.Shop) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName(s),
			Namespace: s.Namespace,
			Labels:    CommonLabels(s.Name),
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"JWT_SECRET":  generateRandomJWTKey(),
			"DB_PASSWORD": "", // CNPG base uses its own secret for db connection.
		},
	}
}

// generateRandomJWTKey generates random 32byte key - encrypts to Base64.
func generateRandomJWTKey() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		// fallback if error in rand generator
		return "fallback-super-secure-key-change-me-in-production"
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
