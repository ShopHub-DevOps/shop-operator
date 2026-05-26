package controller

import (
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"
)

func newDiscordChannel(name, secretName string) *shophubv1alpha1.DiscordChannel {
	return &shophubv1alpha1.DiscordChannel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: shophubv1alpha1.DiscordChannelSpec{
			ChannelName: "alerts-" + name,
			WebhookSecretRef: shophubv1alpha1.DiscordWebhookSecretRef{
				Name: secretName,
			},
			MinSeverity: shophubv1alpha1.DiscordSeverityWarning,
		},
	}
}

func newWebhookSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
}

var _ = Describe("DiscordChannel reconciler", func() {
	Context("when the referenced Secret carries a webhook URL", func() {
		It("reports Ready and records LastValidatedAt", func() {
			secret := newWebhookSecret("hook-ready", map[string][]byte{
				"url": []byte("https://discord.com/api/webhooks/123/abc"),
			})
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, secret) }()

			channel := newDiscordChannel("ready-channel", secret.Name)
			Expect(k8sClient.Create(ctx, channel)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, channel) }()

			Eventually(func() (shophubv1alpha1.DiscordChannelPhase, error) {
				current := &shophubv1alpha1.DiscordChannel{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: channel.Name, Namespace: channel.Namespace}, current); err != nil {
					return "", err
				}
				return current.Status.Phase, nil
			}).Should(Equal(shophubv1alpha1.DiscordChannelPhaseReady))

			current := &shophubv1alpha1.DiscordChannel{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: channel.Name, Namespace: channel.Namespace}, current)).To(Succeed())
			Expect(current.Status.LastValidatedAt).NotTo(BeNil())
		})
	})

	Context("when the referenced Secret does not exist yet", func() {
		It("reports Pending with reason SecretMissing", func() {
			channel := newDiscordChannel("pending-channel", "absent-secret")
			Expect(k8sClient.Create(ctx, channel)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, channel) }()

			Eventually(func() (shophubv1alpha1.DiscordChannelPhase, error) {
				current := &shophubv1alpha1.DiscordChannel{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: channel.Name, Namespace: channel.Namespace}, current); err != nil {
					return "", err
				}
				return current.Status.Phase, nil
			}).Should(Equal(shophubv1alpha1.DiscordChannelPhasePending))
		})
	})

	Context("when the Secret exists but lacks the webhook key", func() {
		It("reports Failed with reason SecretKeyMissing", func() {
			secret := newWebhookSecret("hook-empty", map[string][]byte{
				"other-key": []byte("nope"),
			})
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, secret) }()

			channel := newDiscordChannel("failed-channel", secret.Name)
			Expect(k8sClient.Create(ctx, channel)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, channel) }()

			Eventually(func() (shophubv1alpha1.DiscordChannelPhase, error) {
				current := &shophubv1alpha1.DiscordChannel{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: channel.Name, Namespace: channel.Namespace}, current); err != nil {
					return "", err
				}
				return current.Status.Phase, nil
			}).Should(Equal(shophubv1alpha1.DiscordChannelPhaseFailed))
		})
	})

	Context("OpenAPI validation", func() {
		It("rejects a DiscordChannel with an invalid severity", func() {
			bad := newDiscordChannel("bad-severity", "any-secret")
			bad.Spec.MinSeverity = shophubv1alpha1.DiscordSeverity("emergency")
			err := k8sClient.Create(ctx, bad)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("rejects a DiscordChannel with an empty webhook secret name", func() {
			bad := newDiscordChannel("bad-secret-name", "")
			err := k8sClient.Create(ctx, bad)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})
	})

	Context("on deletion", func() {
		It("releases the finalizer and disappears", func() {
			channel := newDiscordChannel("dying-channel", "absent-secret")
			Expect(k8sClient.Create(ctx, channel)).To(Succeed())

			Eventually(func() bool {
				current := &shophubv1alpha1.DiscordChannel{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: channel.Name, Namespace: channel.Namespace}, current); err != nil {
					return false
				}
				return slices.Contains(current.Finalizers, discordChannelFinalizer)
			}).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, channel)).To(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: channel.Name, Namespace: channel.Namespace}, &shophubv1alpha1.DiscordChannel{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})
	})
})
