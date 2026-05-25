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
})
