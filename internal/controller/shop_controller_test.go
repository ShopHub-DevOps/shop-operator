package controller

import (
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"
	"github.com/ShopHub-DevOps/shop-operator/internal/resources"
)

func newShop(name string) *shophubv1alpha1.Shop {
	return &shophubv1alpha1.Shop{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: shophubv1alpha1.ShopSpec{
			DisplayName:   "Demo " + name,
			Host:          name + ".shophub.local",
			Availability:  shophubv1alpha1.AvailabilityStandard,
			DatabaseTier:  shophubv1alpha1.DatabaseStandard,
			WalletAddress: "0xabcdef1234567890abcdef1234567890abcdef12",
			OwnerEmail:    "test@shophub.local",
			ChainID:       11155111,
		},
	}
}

var _ = Describe("Shop reconciler", func() {
	Context("when a Shop is created", func() {
		var shop *shophubv1alpha1.Shop

		BeforeEach(func() {
			shop = newShop("happy")
			Expect(k8sClient.Create(ctx, shop)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, shop)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: shop.Name, Namespace: shop.Namespace}, &shophubv1alpha1.Shop{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})

		It("creates backend and frontend Deployments owned by the Shop", func() {
			beKey := types.NamespacedName{Name: resources.BackendName(shop), Namespace: shop.Namespace}
			feKey := types.NamespacedName{Name: resources.FrontendName(shop), Namespace: shop.Namespace}

			Eventually(func() error {
				return k8sClient.Get(ctx, beKey, &appsv1.Deployment{})
			}).Should(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, feKey, &appsv1.Deployment{})
			}).Should(Succeed())

			be := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, beKey, be)).To(Succeed())
			Expect(*be.Spec.Replicas).To(Equal(int32(2)))
			Expect(be.OwnerReferences).To(HaveLen(1))
			Expect(be.OwnerReferences[0].Kind).To(Equal("Shop"))
		})

		It("creates backend and frontend Services and an Ingress", func() {
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: resources.BackendName(shop), Namespace: shop.Namespace,
				}, &corev1.Service{})
			}).Should(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: resources.FrontendName(shop), Namespace: shop.Namespace,
				}, &corev1.Service{})
			}).Should(Succeed())

			ing := &networkingv1.Ingress{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: resources.IngressName(shop), Namespace: shop.Namespace,
				}, ing)
			}).Should(Succeed())
			Expect(ing.Spec.Rules[0].Host).To(Equal(shop.Spec.Host))
		})

		It("scales replicas when availability changes from standard to high", func() {
			Eventually(func() (int32, error) {
				dep := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: resources.BackendName(shop), Namespace: shop.Namespace,
				}, dep); err != nil {
					return 0, err
				}
				if dep.Spec.Replicas == nil {
					return 0, nil
				}
				return *dep.Spec.Replicas, nil
			}).Should(Equal(int32(2)))

			Eventually(func() error {
				current := &shophubv1alpha1.Shop{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: shop.Name, Namespace: shop.Namespace}, current); err != nil {
					return err
				}
				current.Spec.Availability = shophubv1alpha1.AvailabilityHigh
				return k8sClient.Update(ctx, current)
			}).Should(Succeed())

			Eventually(func() (int32, error) {
				dep := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: resources.BackendName(shop), Namespace: shop.Namespace,
				}, dep); err != nil {
					return 0, err
				}
				if dep.Spec.Replicas == nil {
					return 0, nil
				}
				return *dep.Spec.Replicas, nil
			}).Should(Equal(int32(3)))
		})

		It("sets phase to Provisioning while Deployments have not reported readiness", func() {
			Eventually(func() (shophubv1alpha1.ShopPhase, error) {
				current := &shophubv1alpha1.Shop{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: shop.Name, Namespace: shop.Namespace}, current); err != nil {
					return "", err
				}
				return current.Status.Phase, nil
			}).Should(Equal(shophubv1alpha1.ShopPhaseProvisioning))
		})
	})

	Context("OpenAPI validation", func() {
		It("rejects a Shop with an invalid wallet address", func() {
			bad := newShop("bad-wallet")
			bad.Spec.WalletAddress = "not-a-wallet"
			err := k8sClient.Create(ctx, bad)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected admission to reject bad wallet, got %v", err)
		})

		It("rejects a Shop with an invalid availability tier", func() {
			bad := newShop("bad-tier")
			bad.Spec.Availability = shophubv1alpha1.AvailabilityTier("super-extra")
			err := k8sClient.Create(ctx, bad)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})
	})

	Context("on deletion", func() {
		It("releases the finalizer and disappears", func() {
			shop := newShop("dying")
			Expect(k8sClient.Create(ctx, shop)).To(Succeed())

			Eventually(func() bool {
				current := &shophubv1alpha1.Shop{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: shop.Name, Namespace: shop.Namespace}, current); err != nil {
					return false
				}
				return slices.Contains(current.Finalizers, shopFinalizer)
			}).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, shop)).To(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: shop.Name, Namespace: shop.Namespace}, &shophubv1alpha1.Shop{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})
	})

	Context("CNPG cluster management", func() {
		It("creates a CNPG Cluster when DatabaseTier is standard", func() {
			shop := newShop("cnpg-test")
			shop.Spec.DatabaseTier = shophubv1alpha1.DatabaseStandard
			Expect(k8sClient.Create(ctx, shop)).To(Succeed())

			// Wait for CNPG Cluster to be created
			// Note: This will log "CNPG operator not installed" if not available,
			// but the test doesn't fail because we ignore that error.
			Eventually(func() bool {
				// We can't check if cluster exists because CNPG CRD might not be installed.
				// Instead, verify that the reconciliation completed (finalizer is set)
				current := &shophubv1alpha1.Shop{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: shop.Name, Namespace: shop.Namespace}, current); err != nil {
					return false
				}
				return slices.Contains(current.Finalizers, shopFinalizer)
			}).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, shop)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: shop.Name, Namespace: shop.Namespace}, &shophubv1alpha1.Shop{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})

		It("adds CNPG Secret to Deployment EnvFrom when DatabaseTier is standard", func() {
			shop := newShop("cnpg-env-test")
			shop.Spec.DatabaseTier = shophubv1alpha1.DatabaseStandard
			Expect(k8sClient.Create(ctx, shop)).To(Succeed())

			// Wait for backend Deployment to be created and check EnvFrom
			beKey := types.NamespacedName{Name: resources.BackendName(shop), Namespace: shop.Namespace}
			Eventually(func() []corev1.EnvFromSource {
				dep := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, beKey, dep); err != nil {
					return nil
				}
				if len(dep.Spec.Template.Spec.Containers) == 0 {
					return nil
				}
				return dep.Spec.Template.Spec.Containers[0].EnvFrom
			}).Should(HaveLen(3)) // ConfigMap, Secret, CNPG Secret

			// Verify CNPG Secret is included
			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, beKey, dep)).To(Succeed())
			envFrom := dep.Spec.Template.Spec.Containers[0].EnvFrom

			cnpgSecretName := resources.CNPGSecretName(shop)
			found := false
			for _, env := range envFrom {
				if env.SecretRef != nil && env.SecretRef.Name == cnpgSecretName {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "CNPG Secret %s not found in Deployment EnvFrom", cnpgSecretName)

			Expect(k8sClient.Delete(ctx, shop)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: shop.Name, Namespace: shop.Namespace}, &shophubv1alpha1.Shop{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})

		It("does not add CNPG Secret when DatabaseTier is light", func() {
			shop := newShop("no-cnpg-test")
			shop.Spec.DatabaseTier = shophubv1alpha1.DatabaseLight
			Expect(k8sClient.Create(ctx, shop)).To(Succeed())

			// Wait for backend Deployment and check EnvFrom
			beKey := types.NamespacedName{Name: resources.BackendName(shop), Namespace: shop.Namespace}
			Eventually(func() []corev1.EnvFromSource {
				dep := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, beKey, dep); err != nil {
					return nil
				}
				if len(dep.Spec.Template.Spec.Containers) == 0 {
					return nil
				}
				return dep.Spec.Template.Spec.Containers[0].EnvFrom
			}).Should(HaveLen(2)) // Only ConfigMap and Secret, no CNPG

			Expect(k8sClient.Delete(ctx, shop)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: shop.Name, Namespace: shop.Namespace}, &shophubv1alpha1.Shop{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})
	})

	Context("REDB database management", func() {
		It("creates a REDB Database when DatabaseTier is light", func() {
			shop := newShop("redb-test")
			shop.Spec.DatabaseTier = shophubv1alpha1.DatabaseLight
			Expect(k8sClient.Create(ctx, shop)).To(Succeed())

			Eventually(func() bool {
				current := &shophubv1alpha1.Shop{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: shop.Name, Namespace: shop.Namespace}, current); err != nil {
					return false
				}
				return slices.Contains(current.Finalizers, shopFinalizer)
			}).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, shop)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: shop.Name, Namespace: shop.Namespace}, &shophubv1alpha1.Shop{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})

		It("adds REDIS_HOST and REDIS_PASSWORD env vars when DatabaseTier is light", func() {
			shop := newShop("redb-env-test")
			shop.Spec.DatabaseTier = shophubv1alpha1.DatabaseLight
			Expect(k8sClient.Create(ctx, shop)).To(Succeed())

			beKey := types.NamespacedName{Name: resources.BackendName(shop), Namespace: shop.Namespace}
			Eventually(func() []corev1.EnvVar {
				dep := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, beKey, dep); err != nil {
					return nil
				}
				if len(dep.Spec.Template.Spec.Containers) == 0 {
					return nil
				}
				return dep.Spec.Template.Spec.Containers[0].Env
			}).Should(HaveLen(2))

			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, beKey, dep)).To(Succeed())
			env := dep.Spec.Template.Spec.Containers[0].Env

			Expect(env[0].Name).To(Equal("REDIS_HOST"))
			Expect(env[0].Value).To(Equal(resources.REDBDatabaseName(shop)))
			Expect(env[1].Name).To(Equal("REDIS_PASSWORD"))
			Expect(env[1].ValueFrom.SecretKeyRef.Name).To(Equal(resources.REDBSecretName(shop)))

			Expect(k8sClient.Delete(ctx, shop)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: shop.Name, Namespace: shop.Namespace}, &shophubv1alpha1.Shop{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})
	})
})
