package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"
	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
)

const (
	discordChannelFinalizer = "shophub.io/discordchannel-finalizer"
	defaultWebhookSecretKey = "url"
)

// DiscordChannelReconciler reconciles a DiscordChannel by validating that the
// referenced webhook Secret is present and well-formed, and by generating the
// AlertmanagerConfig that routes matching alerts to that Discord webhook.
//
// The webhook URL is never copied into the CR or into any generated object - the
// AlertmanagerConfig references the Secret, and Alertmanager itself performs the
// delivery. Alert rules come from the ShopReconciler (PrometheusRule), and the
// `tenant` / `severity` labels they carry are what the generated route matches on.
type DiscordChannelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=shophub.io,resources=discordchannels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=shophub.io,resources=discordchannels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=shophub.io,resources=discordchannels/finalizers,verbs=update
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=alertmanagerconfigs,verbs=get;list;watch;create;update;patch;delete

func (r *DiscordChannelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	channel := &shophubv1alpha1.DiscordChannel{}
	if err := r.Get(ctx, req.NamespacedName, channel); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !channel.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(channel, discordChannelFinalizer) {
			logger.Info("releasing DiscordChannel finalizer", "channel", channel.Name)
			controllerutil.RemoveFinalizer(channel, discordChannelFinalizer)
			if err := r.Update(ctx, channel); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(channel, discordChannelFinalizer) {
		controllerutil.AddFinalizer(channel, discordChannelFinalizer)
		if err := r.Update(ctx, channel); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	phase, condStatus, condReason, condMessage := r.validateWebhookSecret(ctx, channel)

	channel.Status.Phase = phase
	channel.Status.ObservedGeneration = channel.Generation
	if phase == shophubv1alpha1.DiscordChannelPhaseReady {
		now := metav1.NewTime(time.Now())
		channel.Status.LastValidatedAt = &now
	}
	meta.SetStatusCondition(&channel.Status.Conditions, metav1.Condition{
		Type:               conditionReady,
		Status:             condStatus,
		Reason:             condReason,
		Message:            condMessage,
		ObservedGeneration: channel.Generation,
	})

	if err := r.Status().Update(ctx, channel); err != nil {
		return ctrl.Result{}, fmt.Errorf("status: %w", err)
	}

	if phase == shophubv1alpha1.DiscordChannelPhaseReady {
		if err := r.reconcileAlertmanagerConfig(ctx, channel, channel.Spec.WebhookSecretRef.Key); err != nil {
			logger.Error(err, "failed to reconcile AlertmanagerConfig")
			return ctrl.Result{}, err
		}
	} else {
		if err := r.deleteAlertmanagerConfig(ctx, channel); err != nil {
			logger.Error(err, "failed to delete AlertmanagerConfig")
			return ctrl.Result{}, err
		}
	}

	logger.V(1).Info("reconciled DiscordChannel", "channel", channel.Spec.ChannelName, "phase", phase)
	return ctrl.Result{}, nil
}

// validateWebhookSecret loads the Secret named in the spec and verifies it
// carries a non-empty value under the configured key. Returns the phase and
// condition fields that should be written to the status.
func (r *DiscordChannelReconciler) validateWebhookSecret(ctx context.Context, channel *shophubv1alpha1.DiscordChannel) (
	shophubv1alpha1.DiscordChannelPhase, metav1.ConditionStatus, string, string,
) {
	key := channel.Spec.WebhookSecretRef.Key
	if key == "" {
		key = defaultWebhookSecretKey
	}

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: channel.Namespace,
		Name:      channel.Spec.WebhookSecretRef.Name,
	}, secret)
	if apierrors.IsNotFound(err) {
		return shophubv1alpha1.DiscordChannelPhasePending,
			metav1.ConditionFalse,
			"SecretMissing",
			fmt.Sprintf("Secret %q not found in namespace %q", channel.Spec.WebhookSecretRef.Name, channel.Namespace)
	}
	if err != nil {
		return shophubv1alpha1.DiscordChannelPhaseFailed,
			metav1.ConditionFalse,
			"SecretLookupFailed",
			fmt.Sprintf("Failed to read Secret %q: %v", channel.Spec.WebhookSecretRef.Name, err)
	}

	value, ok := secret.Data[key]
	if !ok || len(value) == 0 {
		return shophubv1alpha1.DiscordChannelPhaseFailed,
			metav1.ConditionFalse,
			"SecretKeyMissing",
			fmt.Sprintf("Secret %q has no value under key %q", channel.Spec.WebhookSecretRef.Name, key)
	}

	return shophubv1alpha1.DiscordChannelPhaseReady,
		metav1.ConditionTrue,
		"WebhookSecretValid",
		fmt.Sprintf("Secret %q carries a webhook URL under key %q", channel.Spec.WebhookSecretRef.Name, key)
}

func (r *DiscordChannelReconciler) reconcileAlertmanagerConfig(ctx context.Context, channel *shophubv1alpha1.DiscordChannel, secretKey string) error {
	amConfig := &monitoringv1alpha1.AlertmanagerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      channel.Name,
			Namespace: channel.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, amConfig, func() error {
		if err := controllerutil.SetControllerReference(channel, amConfig, r.Scheme); err != nil {
			return err
		}

		// Alertmanager only merges AlertmanagerConfig objects that match its
		// alertmanagerConfigSelector. kube-prometheus-stack is deployed with
		// `alertmanagerConfigSelector: {alertmanagerConfig: shophub}`, so without
		// this label the generated routing is silently ignored and no alert ever
		// reaches Discord. Mirrors the `release: shophub` label the ShopReconciler
		// stamps on ServiceMonitors for the same reason.
		amConfig.SetLabels(map[string]string{
			"app.kubernetes.io/instance": channel.Name,
			"app.kubernetes.io/name":     "discordchannel",
			"app.kubernetes.io/part-of":  "shophub-platform",
			"alertmanagerConfig":         "shophub",
		})

		amConfig.Spec = monitoringv1alpha1.AlertmanagerConfigSpec{
			Receivers: []monitoringv1alpha1.Receiver{
				{
					Name: "discord",
					DiscordConfigs: []monitoringv1alpha1.DiscordConfig{
						{
							APIURL: corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: channel.Spec.WebhookSecretRef.Name,
								},
								Key: secretKey,
							},
						},
					},
				},
			},
			Route: &monitoringv1alpha1.Route{
				Receiver: "discord",
				GroupBy:  []string{"alertname", "job"},
			},
		}

		var matchers []monitoringv1alpha1.Matcher

		if channel.Spec.ShopRef != "" {
			matchers = append(matchers, monitoringv1alpha1.Matcher{
				Name:      "tenant",
				Value:     channel.Spec.ShopRef,
				MatchType: monitoringv1alpha1.MatchType("="),
			})
		}

		severityRegex := "warning|critical"
		switch channel.Spec.MinSeverity {
		case shophubv1alpha1.DiscordSeverityInfo:
			severityRegex = "info|warning|critical"
		case shophubv1alpha1.DiscordSeverityCritical:
			severityRegex = "critical"
		}

		matchers = append(matchers, monitoringv1alpha1.Matcher{
			Name:      "severity",
			Value:     severityRegex,
			MatchType: monitoringv1alpha1.MatchType("=~"),
		})

		amConfig.Spec.Route.Matchers = matchers

		return nil
	})

	return err
}

func (r *DiscordChannelReconciler) deleteAlertmanagerConfig(ctx context.Context, channel *shophubv1alpha1.DiscordChannel) error {
	amConfig := &monitoringv1alpha1.AlertmanagerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      channel.Name,
			Namespace: channel.Namespace,
		},
	}
	err := r.Delete(ctx, amConfig)
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	return nil
}

func (r *DiscordChannelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shophubv1alpha1.DiscordChannel{}).
		Owns(&monitoringv1alpha1.AlertmanagerConfig{}).
		Named("discordchannel").
		Complete(r)
}
