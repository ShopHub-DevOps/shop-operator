package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"
)

const walletFinalizer = "shophub.io/wallet-finalizer"

// WalletReconciler reconciles a Wallet. The CR is purely declarative metadata
// (address + chain + purpose), so the reconciler's job is light: maintain the
// finalizer hook and mark the Wallet Ready once OpenAPI validation has passed.
// On-chain balance polling and payout integration are left for later issues.
type WalletReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=shophub.io,resources=wallets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=shophub.io,resources=wallets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=shophub.io,resources=wallets/finalizers,verbs=update

func (r *WalletReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	wallet := &shophubv1alpha1.Wallet{}
	if err := r.Get(ctx, req.NamespacedName, wallet); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !wallet.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(wallet, walletFinalizer) {
			logger.Info("releasing Wallet finalizer", "wallet", wallet.Name)
			controllerutil.RemoveFinalizer(wallet, walletFinalizer)
			if err := r.Update(ctx, wallet); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(wallet, walletFinalizer) {
		controllerutil.AddFinalizer(wallet, walletFinalizer)
		if err := r.Update(ctx, wallet); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	wallet.Status.Phase = shophubv1alpha1.WalletPhaseReady
	wallet.Status.ObservedGeneration = wallet.Generation
	meta.SetStatusCondition(&wallet.Status.Conditions, metav1.Condition{
		Type:               conditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "WalletRegistered",
		Message:            fmt.Sprintf("Wallet %s on chain %d registered as %s", wallet.Spec.Address, wallet.Spec.ChainID, wallet.Spec.Purpose),
		ObservedGeneration: wallet.Generation,
	})

	if err := r.Status().Update(ctx, wallet); err != nil {
		return ctrl.Result{}, fmt.Errorf("status: %w", err)
	}

	logger.V(1).Info("reconciled Wallet", "wallet", wallet.Name, "phase", wallet.Status.Phase)
	return ctrl.Result{}, nil
}

func (r *WalletReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shophubv1alpha1.Wallet{}).
		Named("wallet").
		Complete(r)
}
