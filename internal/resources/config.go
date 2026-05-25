package resources

import (
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
		},
	}
}

// BuildSecret holds placeholders for sensitive values (JWT signing key,
// database password). The actual contents are populated by separate
// reconcilers - this builder establishes the resource shape so envFrom
// references are stable across reconciles.
func BuildSecret(s *shophubv1alpha1.Shop) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName(s),
			Namespace: s.Namespace,
			Labels:    CommonLabels(s.Name),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"JWT_SECRET":  []byte(""),
			"DB_PASSWORD": []byte(""),
		},
	}
}
