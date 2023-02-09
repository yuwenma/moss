package controllers

import (
	addonsv1alpha1 "acp.git.corp.google.com/moss/api/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/addon"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/addon/pkg/status"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/declarative"
)

var _ reconcile.Reconciler = &ArgoCDReconciler{}

// ArgoCDReconciler reconciles a ArgoCD object
type ArgoCDReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	declarative.Reconciler
}

//+kubebuilder:rbac:groups=addons.configdelivery.anthos.io,resources=argocds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=addons.configdelivery.anthos.io,resources=argocds/status,verbs=get;update;patch

// SetupWithManager sets up the controller with the Manager.
func (r *ArgoCDReconciler) SetupWithManager(mgr ctrl.Manager) error {
	addon.Init()

	labels := map[string]string{
		"k8s-app": "argocd",
	}

	watchLabels := declarative.SourceLabel(mgr.GetScheme())
	if err := r.Reconciler.Init(mgr, &addonsv1alpha1.ArgoCD{},
		declarative.WithObjectTransform(declarative.AddLabels(labels)),
		declarative.WithOwner(declarative.SourceAsOwner),
		declarative.WithLabels(watchLabels),
		declarative.WithStatus(status.NewKstatusCheck(mgr.GetClient(), &r.Reconciler)),
		declarative.WithObjectTransform(addon.ApplyPatches),
	); err != nil {
		return err
	}

	c, err := controller.New("argocd-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ArgoCD
	err = c.Watch(&source.Kind{Type: &addonsv1alpha1.ArgoCD{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to deployed objects
	childOptions := declarative.WatchChildrenOptions{
		ScopeWatchesToNamespace: true,
		Manager:                 mgr,
		RESTConfig:              mgr.GetConfig(),
		LabelMaker:              watchLabels,
		Controller:              c,
		Reconciler:              r,
	}
	_, err = declarative.WatchChildren(childOptions)
	if err != nil {
		return err
	}

	return nil
}
