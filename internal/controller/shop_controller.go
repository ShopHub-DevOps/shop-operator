package controller

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"
	"github.com/ShopHub-DevOps/shop-operator/internal/resources"
)

const (
	conditionReady = "Ready"
	shopFinalizer  = "shophub.io/shop-finalizer"
)

// ShopReconciler reconciles a Shop object into a per-tenant Deployment + Service + Ingress stack.
type ShopReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=shophub.io,resources=shops,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=shophub.io,resources=shops/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=shophub.io,resources=shops/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services;configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors;prometheusrules,verbs=get;list;watch;create;update;patch;delete

func (r *ShopReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	shop := &shophubv1alpha1.Shop{}
	if err := r.Get(ctx, req.NamespacedName, shop); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Deletion path: child resources are reclaimed by OwnerReference cascade,
	// so the finalizer hook just logs and yields. External cleanup hooks
	// (CNPG cluster, REDB instance) will plug in here in shop-operator#10 / #11.
	// Deletion path: wait for CNPG Cluster to be deleted before removing finalizer
	if !shop.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(shop, shopFinalizer) {
			// Wait for CNPG Cluster to be deleted if database tier is standard
			if shop.Spec.DatabaseTier == shophubv1alpha1.DatabaseStandard {
				cluster := &unstructured.Unstructured{}
				cluster.SetAPIVersion("postgresql.cnpg.io/v1")
				cluster.SetKind("Cluster")
				err := r.Get(ctx, types.NamespacedName{
					Name:      resources.CNPGClusterName(shop),
					Namespace: shop.Namespace,
				}, cluster)

				if err == nil {
					// Cluster still exists, wait for it to be deleted
					logger.Info("waiting for CNPG cluster to be deleted", "shop", shop.Name)
					return ctrl.Result{Requeue: true}, nil
				}

				if !apierrors.IsNotFound(err) && !strings.Contains(err.Error(), "no matches for kind") {
					// Actual error, not just not found
					return ctrl.Result{}, fmt.Errorf("check cnpg cluster: %w", err)
				}
				// Cluster is deleted, proceed with finalizer removal
			}

			// Wait for REDB deletion if light tier
			if shop.Spec.DatabaseTier == shophubv1alpha1.DatabaseLight {
				redb := &unstructured.Unstructured{}
				redb.SetAPIVersion("app.redislabs.com/v1alpha1")
				redb.SetKind("RedisEnterpriseDatabase")
				err := r.Get(ctx, types.NamespacedName{
					Name:      resources.REDBDatabaseName(shop),
					Namespace: shop.Namespace,
				}, redb)

				if err == nil {
					logger.Info("waiting for REDB database to be deleted", "shop", shop.Name)
					return ctrl.Result{Requeue: true}, nil
				}
				if !apierrors.IsNotFound(err) && !strings.Contains(err.Error(), "no matches for kind") {
					return ctrl.Result{}, fmt.Errorf("check redb database: %w", err)
				}
			}

			logger.Info("releasing Shop finalizer", "shop", shop.Name)
			controllerutil.RemoveFinalizer(shop, shopFinalizer)
			if err := r.Update(ctx, shop); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(shop, shopFinalizer) {
		controllerutil.AddFinalizer(shop, shopFinalizer)
		if err := r.Update(ctx, shop); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// r.logDatabaseTierStub(ctx, shop)
	if err := r.reconcileCNPGCluster(ctx, shop); err != nil {
		return ctrl.Result{}, fmt.Errorf("cnpg cluster: %w", err)
	}

	if err := r.reconcileREDBDatabase(ctx, shop); err != nil {
		return ctrl.Result{}, fmt.Errorf("redb database: %w", err)
	}

	if err := r.reconcileConfigMap(ctx, shop); err != nil {
		return ctrl.Result{}, fmt.Errorf("configmap: %w", err)
	}
	if err := r.reconcileSecret(ctx, shop); err != nil {
		return ctrl.Result{}, fmt.Errorf("secret: %w", err)
	}
	if err := r.reconcileBackendDeployment(ctx, shop); err != nil {
		return ctrl.Result{}, fmt.Errorf("backend deployment: %w", err)
	}
	if err := r.reconcileFrontendDeployment(ctx, shop); err != nil {
		return ctrl.Result{}, fmt.Errorf("frontend deployment: %w", err)
	}
	if err := r.reconcileBackendService(ctx, shop); err != nil {
		return ctrl.Result{}, fmt.Errorf("backend service: %w", err)
	}
	if err := r.reconcileFrontendService(ctx, shop); err != nil {
		return ctrl.Result{}, fmt.Errorf("frontend service: %w", err)
	}
	if err := r.reconcileServiceMonitor(ctx, shop); err != nil {
		return ctrl.Result{}, fmt.Errorf("servicemonitor: %w", err)
	}
	if err := r.reconcilePrometheusRule(ctx, shop); err != nil {
		return ctrl.Result{}, fmt.Errorf("prometheusrule: %w", err)
	}
	if err := r.reconcileIngress(ctx, shop); err != nil {
		return ctrl.Result{}, fmt.Errorf("ingress: %w", err)
	}

	if err := r.reconcileStatus(ctx, shop); err != nil {
		return ctrl.Result{}, fmt.Errorf("status: %w", err)
	}

	logger.V(1).Info("reconciled Shop", "host", shop.Spec.Host, "phase", shop.Status.Phase)
	return ctrl.Result{}, nil
}

// reconcileStatus derives the Shop's phase from its child Deployments and
// writes it back via the status subresource.
func (r *ShopReconciler) reconcileStatus(ctx context.Context, shop *shophubv1alpha1.Shop) error {
	desiredReplicas := resources.Replicas(shop)

	backendReady, err := r.deploymentReady(ctx, shop.Namespace, resources.BackendName(shop), desiredReplicas)
	if err != nil {
		return err
	}
	frontendReady, err := r.deploymentReady(ctx, shop.Namespace, resources.FrontendName(shop), desiredReplicas)
	if err != nil {
		return err
	}

	phase := shophubv1alpha1.ShopPhaseProvisioning
	condStatus := metav1.ConditionFalse
	condReason := "DeploymentsProgressing"
	condMessage := "Waiting for backend and frontend Deployments to become Available"
	if backendReady && frontendReady {
		phase = shophubv1alpha1.ShopPhaseReady
		condStatus = metav1.ConditionTrue
		condReason = "AllDeploymentsReady"
		condMessage = "Backend and frontend Deployments report all replicas Available"
	}

	shop.Status.Phase = phase
	shop.Status.URL = fmt.Sprintf("https://%s", shop.Spec.Host)
	shop.Status.ObservedGeneration = shop.Generation
	meta.SetStatusCondition(&shop.Status.Conditions, metav1.Condition{
		Type:               conditionReady,
		Status:             condStatus,
		Reason:             condReason,
		Message:            condMessage,
		ObservedGeneration: shop.Generation,
	})

	return r.Status().Update(ctx, shop)
}

// deploymentReady reports whether the named Deployment has reached the
// desired replica count with all replicas Available. Returns (false, nil)
// when the Deployment does not yet exist (treated as still provisioning).
func (r *ShopReconciler) deploymentReady(ctx context.Context, namespace, name string, desired int32) (bool, error) {
	dep := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, dep)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return dep.Status.AvailableReplicas == desired && dep.Status.ObservedGeneration >= dep.Generation, nil
}

// logDatabaseTierStub is a placeholder for the real CNPG / REDB provisioning,
// tracked in shop-operator#10 (CNPG) and shop-operator#11 (REDB).
/*
func (r *ShopReconciler) logDatabaseTierStub(ctx context.Context, shop *shophubv1alpha1.Shop) {
	logger := log.FromContext(ctx)
	switch shop.Spec.DatabaseTier {
	case shophubv1alpha1.DatabaseStandard:
		logger.V(1).Info("database tier stub: would provision CNPG cluster", "shop", shop.Name)
	case shophubv1alpha1.DatabaseLight:
		logger.V(1).Info("database tier stub: would provision Redis via REDB", "shop", shop.Name)
	}
}
*/

func (r *ShopReconciler) reconcileCNPGCluster(ctx context.Context, shop *shophubv1alpha1.Shop) error {
	if shop.Spec.DatabaseTier != shophubv1alpha1.DatabaseStandard {
		return nil
	}

	desired := resources.BuildCNPGCluster(shop)
	current := &unstructured.Unstructured{}
	current.SetAPIVersion("postgresql.cnpg.io/v1")
	current.SetKind("Cluster")
	current.SetName(desired.GetName())
	current.SetNamespace(desired.GetNamespace())

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, current, func() error {
		current.SetLabels(desired.GetLabels())
		current.Object["spec"] = desired.Object["spec"]
		return controllerutil.SetControllerReference(shop, current, r.Scheme)
	})

	// Ignore "no matches for kind" error — CNPG operator may not be installed
	if err != nil && strings.Contains(err.Error(), "no matches for kind") {
		logger := log.FromContext(ctx)
		logger.Info("CNPG operator not installed, skipping cluster creation", "shop", shop.Name)
		return nil
	}

	return err
}

func (r *ShopReconciler) reconcileConfigMap(ctx context.Context, shop *shophubv1alpha1.Shop) error {
	desired := resources.BuildConfigMap(shop)
	current := &corev1.ConfigMap{}
	current.Name = desired.Name
	current.Namespace = desired.Namespace

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, current, func() error {
		current.Labels = desired.Labels
		current.Data = desired.Data
		return controllerutil.SetControllerReference(shop, current, r.Scheme)
	})
	return err
}

func (r *ShopReconciler) reconcileSecret(ctx context.Context, shop *shophubv1alpha1.Shop) error {
	// Fetch shared JWT secret
	sharedSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: "shophub-secrets", Namespace: "default"}, sharedSecret)
	if err != nil {
		return fmt.Errorf("failed to get shared secret: %w", err)
	}
	jwtSecretBytes, ok := sharedSecret.Data["jwt-secret"]
	if !ok {
		return fmt.Errorf("shared secret missing jwt-secret key")
	}

	desired := resources.BuildSecret(shop, string(jwtSecretBytes))
	current := &corev1.Secret{}
	current.Name = desired.Name
	current.Namespace = desired.Namespace

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, current, func() error {
		current.Labels = desired.Labels
		current.Type = desired.Type
		// Preserve existing values so we don't clobber secrets populated by
		// other reconcilers (e.g. CNPG bootstrap secret). Only seed empty keys.
		if current.Data == nil {
			current.Data = map[string][]byte{}
		}
		for k, v := range desired.StringData {
			if _, exists := current.Data[k]; !exists {
				current.Data[k] = []byte(v)
			}
		}
		return controllerutil.SetControllerReference(shop, current, r.Scheme)
	})
	return err
}

func (r *ShopReconciler) reconcileBackendDeployment(ctx context.Context, shop *shophubv1alpha1.Shop) error {
	return r.applyDeployment(ctx, shop, resources.BuildBackendDeployment(shop))
}

func (r *ShopReconciler) reconcileFrontendDeployment(ctx context.Context, shop *shophubv1alpha1.Shop) error {
	return r.applyDeployment(ctx, shop, resources.BuildFrontendDeployment(shop))
}

func (r *ShopReconciler) applyDeployment(ctx context.Context, shop *shophubv1alpha1.Shop, desired *appsv1.Deployment) error {
	current := &appsv1.Deployment{}
	current.Name = desired.Name
	current.Namespace = desired.Namespace

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, current, func() error {
		current.Labels = desired.Labels
		// Only overwrite spec; never touch status.
		if !equality.Semantic.DeepEqual(current.Spec, desired.Spec) {
			current.Spec = desired.Spec
		}
		return controllerutil.SetControllerReference(shop, current, r.Scheme)
	})
	return err
}

func (r *ShopReconciler) reconcileBackendService(ctx context.Context, shop *shophubv1alpha1.Shop) error {
	return r.applyService(ctx, shop, resources.BuildBackendService(shop))
}

func (r *ShopReconciler) reconcileFrontendService(ctx context.Context, shop *shophubv1alpha1.Shop) error {
	return r.applyService(ctx, shop, resources.BuildFrontendService(shop))
}

func (r *ShopReconciler) applyService(ctx context.Context, shop *shophubv1alpha1.Shop, desired *corev1.Service) error {
	current := &corev1.Service{}
	current.Name = desired.Name
	current.Namespace = desired.Namespace

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, current, func() error {
		current.Labels = desired.Labels
		current.Spec.Type = desired.Spec.Type
		current.Spec.Selector = desired.Spec.Selector
		current.Spec.Ports = desired.Spec.Ports
		return controllerutil.SetControllerReference(shop, current, r.Scheme)
	})
	return err
}

func (r *ShopReconciler) reconcileIngress(ctx context.Context, shop *shophubv1alpha1.Shop) error {
	desired := resources.BuildIngress(shop)
	current := &networkingv1.Ingress{}
	current.Name = desired.Name
	current.Namespace = desired.Namespace

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, current, func() error {
		current.Labels = desired.Labels
		current.Annotations = desired.Annotations
		current.Spec = desired.Spec
		return controllerutil.SetControllerReference(shop, current, r.Scheme)
	})
	return err
}

func (r *ShopReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shophubv1alpha1.Shop{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&networkingv1.Ingress{}).
		// Owns(&unstructured.Unstructured{}).
		Named("shop").
		Complete(r)
}

func (r *ShopReconciler) reconcileREDBDatabase(ctx context.Context, shop *shophubv1alpha1.Shop) error {
	if shop.Spec.DatabaseTier != shophubv1alpha1.DatabaseLight {
		return nil
	}

	desired := resources.BuildREDBDatabase(shop)
	current := &unstructured.Unstructured{}
	current.SetAPIVersion("app.redislabs.com/v1alpha1")
	current.SetKind("RedisEnterpriseDatabase")
	current.SetName(desired.GetName())
	current.SetNamespace(desired.GetNamespace())

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, current, func() error {
		current.SetLabels(desired.GetLabels())
		current.Object["spec"] = desired.Object["spec"]
		return controllerutil.SetControllerReference(shop, current, r.Scheme)
	})

	// Ignore "no matches for kind" error — REDB operator may not be installed
	if err != nil && strings.Contains(err.Error(), "no matches for kind") {
		logger := log.FromContext(ctx)
		logger.Info("REDB operator not installed, skipping database creation", "shop", shop.Name)
		return nil
	}

	return err
}

func (r *ShopReconciler) reconcileServiceMonitor(ctx context.Context, shop *shophubv1alpha1.Shop) error {
	sm := &unstructured.Unstructured{}
	sm.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Version: "v1",
		Kind:    "ServiceMonitor",
	})
	sm.SetName(resources.BackendName(shop) + "-backend")
	sm.SetNamespace(shop.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sm, func() error {
		sm.SetLabels(map[string]string{
			"app.kubernetes.io/instance":   shop.Name,
			"app.kubernetes.io/managed-by": "Helm",
			"app.kubernetes.io/name":       "shop",
			"app.kubernetes.io/part-of":    "shophub-platform",
			"release":                      "shophub",
		})

		err := controllerutil.SetControllerReference(shop, sm, r.Scheme)
		if err != nil {
			return err
		}

		spec := map[string]any{
			"selector": map[string]any{
				"matchLabels": map[string]any{
					"app.kubernetes.io/component": "backend",
					"app.kubernetes.io/instance":  shop.Name,
					"app.kubernetes.io/name":      "shop",
				},
			},
			"endpoints": []any{
				map[string]any{
					"port": "http",
					"path": "/metrics",
				},
			},
			"podTargetLabels": []interface{}{
				"shophub.io/host",
			},
		}
		sm.Object["spec"] = spec
		return nil
	})

	// Ignore "no matches for kind" error if prometheus operator CRDs are not installed
	if err != nil && strings.Contains(err.Error(), "no matches for kind") {
		logger := log.FromContext(ctx)
		logger.Info("ServiceMonitor CRD not installed, skipping ServiceMonitor creation", "shop", shop.Name)
		return nil
	}

	return err
}

func (r *ShopReconciler) reconcilePrometheusRule(ctx context.Context, shop *shophubv1alpha1.Shop) error {
	pr := &unstructured.Unstructured{}
	pr.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Version: "v1",
		Kind:    "PrometheusRule",
	})
	pr.SetName(shop.Name + "-tenant-rules")
	pr.SetNamespace(shop.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pr, func() error {
		pr.SetLabels(map[string]string{
			"app.kubernetes.io/instance":   shop.Name,
			"app.kubernetes.io/managed-by": "Helm",
			"app.kubernetes.io/name":       "shop",
			"app.kubernetes.io/part-of":    "shophub-platform",
			"release":                      "shophub",
		})

		err := controllerutil.SetControllerReference(shop, pr, r.Scheme)
		if err != nil {
			return err
		}

		spec := map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{
					"name": shop.Name + ".rules",
					"rules": []interface{}{
						map[string]interface{}{
							"alert": "HighErrorRate",
							"expr":  fmt.Sprintf(`sum(rate(shop_http_requests_total{status_code=~"5..", pod=~"%s-.*"}[5m])) / sum(rate(shop_http_requests_total{pod=~"%s-.*"}[5m])) > 0.05`, shop.Name, shop.Name),
							"for":   "5m",
							"labels": map[string]interface{}{
								"severity":  "critical",
								"tenant":    shop.Name,
								"namespace": shop.Namespace,
							},
							"annotations": map[string]interface{}{
								"summary":     fmt.Sprintf("Tenant %s has a high error rate", shop.Name),
								"description": fmt.Sprintf("Error rate for tenant %s has exceeded the threshold.", shop.Name),
							},
						},
						map[string]interface{}{
							"alert": "HighLatency",
							"expr":  fmt.Sprintf(`histogram_quantile(0.95, sum(rate(shop_http_request_duration_seconds_bucket{pod=~"%s-.*"}[5m])) by (le)) > 2.0`, shop.Name),
							"for":   "5m",
							"labels": map[string]interface{}{
								"severity":  "warning",
								"tenant":    shop.Name,
								"namespace": shop.Namespace,
							},
							"annotations": map[string]interface{}{
								"summary":     fmt.Sprintf("Tenant %s is experiencing high latency", shop.Name),
								"description": fmt.Sprintf("95th percentile latency for tenant %s is above 2.0s.", shop.Name),
							},
						},
						map[string]interface{}{
							"alert": "Frequent4xx",
							"expr":  fmt.Sprintf(`sum(rate(shop_http_requests_total{status_code=~"4..", pod=~"%s-.*"}[5m])) by (route) > 10`, shop.Name),
							"for":   "5m",
							"labels": map[string]interface{}{
								"severity":  "warning",
								"tenant":    shop.Name,
								"namespace": shop.Namespace,
							},
							"annotations": map[string]interface{}{
								"summary":     fmt.Sprintf("Frequent 4xx errors on {{ $labels.route }} for %s", shop.Name),
								"description": fmt.Sprintf("Route {{ $labels.route }} for tenant %s is experiencing a high rate of 4xx errors.", shop.Name),
							},
						},
						map[string]interface{}{
							"alert": "TenantResourceSaturation",
							"expr":  fmt.Sprintf(`sum by (pod) (container_memory_usage_bytes{pod=~"%s-.*", container!="POD"}) / sum by (pod) (kube_pod_container_resource_limits{resource="memory", pod=~"%s-.*"}) > 0.85`, shop.Name, shop.Name),
							"for":   "5m",
							"labels": map[string]interface{}{
								"severity":  "warning",
								"tenant":    shop.Name,
								"namespace": shop.Namespace,
							},
							"annotations": map[string]interface{}{
								"summary":     "Pod {{ $labels.pod }} Memory saturation",
								"description": fmt.Sprintf("Pod {{ $labels.pod }} for tenant %s is approaching its memory limit.", shop.Name),
							},
						},
					},
				},
			},
		}
		pr.Object["spec"] = spec
		return nil
	})

	// Ignore "no matches for kind" error if prometheus operator CRDs are not installed
	if err != nil && strings.Contains(err.Error(), "no matches for kind") {
		logger := log.FromContext(ctx)
		logger.Info("PrometheusRule CRD not installed, skipping PrometheusRule creation", "shop", shop.Name)
		return nil
	}

	return err
}
