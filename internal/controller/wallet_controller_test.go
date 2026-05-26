package controller

import (
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"
)

func newWallet(name string) *shophubv1alpha1.Wallet {
	return &shophubv1alpha1.Wallet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: shophubv1alpha1.WalletSpec{
			DisplayName: "Wallet " + name,
			Address:     "0x1234567890abcdef1234567890abcdef12345678",
			ChainID:     11155111,
			Purpose:     shophubv1alpha1.WalletPurposePayments,
		},
	}
}

var _ = Describe("Wallet reconciler", func() {
	Context("when a valid Wallet is created", func() {
		It("reports Ready and records ObservedGeneration", func() {
			wallet := newWallet("happy-wallet")
			Expect(k8sClient.Create(ctx, wallet)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, wallet) }()

			Eventually(func() (shophubv1alpha1.WalletPhase, error) {
				current := &shophubv1alpha1.Wallet{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: wallet.Name, Namespace: wallet.Namespace}, current); err != nil {
					return "", err
				}
				return current.Status.Phase, nil
			}).Should(Equal(shophubv1alpha1.WalletPhaseReady))

			current := &shophubv1alpha1.Wallet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: wallet.Name, Namespace: wallet.Namespace}, current)).To(Succeed())
			Expect(current.Status.ObservedGeneration).To(Equal(current.Generation))
		})
	})

	Context("OpenAPI validation", func() {
		It("rejects a Wallet with an invalid address", func() {
			bad := newWallet("bad-address")
			bad.Spec.Address = "definitely-not-a-wallet"
			err := k8sClient.Create(ctx, bad)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("rejects a Wallet with an unknown purpose", func() {
			bad := newWallet("bad-purpose")
			bad.Spec.Purpose = shophubv1alpha1.WalletPurpose("speculation")
			err := k8sClient.Create(ctx, bad)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("rejects a Wallet with an empty displayName", func() {
			bad := newWallet("bad-display")
			bad.Spec.DisplayName = ""
			err := k8sClient.Create(ctx, bad)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("defaults chainId to Sepolia when omitted", func() {
			wallet := newWallet("default-chain")
			wallet.Spec.ChainID = 0
			Expect(k8sClient.Create(ctx, wallet)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, wallet) }()

			current := &shophubv1alpha1.Wallet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: wallet.Name, Namespace: wallet.Namespace}, current)).To(Succeed())
			Expect(current.Spec.ChainID).To(Equal(int64(11155111)))
		})
	})

	Context("on deletion", func() {
		It("releases the finalizer and disappears", func() {
			wallet := newWallet("dying-wallet")
			Expect(k8sClient.Create(ctx, wallet)).To(Succeed())

			Eventually(func() bool {
				current := &shophubv1alpha1.Wallet{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: wallet.Name, Namespace: wallet.Namespace}, current); err != nil {
					return false
				}
				return slices.Contains(current.Finalizers, walletFinalizer)
			}).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, wallet)).To(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: wallet.Name, Namespace: wallet.Namespace}, &shophubv1alpha1.Wallet{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})
	})
})
