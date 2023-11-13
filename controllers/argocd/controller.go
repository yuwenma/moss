package argocd

import (
	"context"

	addonsv1alpha1 "github.com/yuwenma/moss/moss/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/addon"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/addon/pkg/status"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/declarative"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/declarative/pkg/applier"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/declarative/pkg/manifest"
)

const (
	ArgoCDNamespace = "argocd"
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

	// TODO: use the applyset recommended labels.
	labels := map[string]string{
		"k8s-app": "argocd",
	}
	// TODO: (k-d-p side) Need the applyset prune logic for this applier.
	applier := applier.NewApplySetApplier(metav1.PatchOptions{}, metav1.DeleteOptions{}, applier.ApplysetOptions{})
	watchLabels := declarative.SourceLabel(mgr.GetScheme())

	if err := r.Reconciler.Init(mgr, &addonsv1alpha1.ArgoCD{},
		declarative.WithLabels(watchLabels),
		declarative.WithObjectTransform(declarative.AddLabels(labels)),
		declarative.WithOwner(declarative.SourceAsOwner),
		declarative.WithObjectTransform(
			declarative.AddLabels(labels),
		),
		// TODO: Define `ArgoCD.Status` to ack users the health status; k-d-p side needs to extend the kstatus support.
		declarative.WithStatus(status.NewKstatusCheck(mgr.GetClient(), &r.Reconciler)),
		declarative.WithObjectTransform(addon.ApplyPatches),
		declarative.WithApplier(applier),
		declarative.WithApplyPrune(),
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

// namespaceExist guarantees the `ArgoCD` namespace exist in the cluster. This is not necessary if the deployment manifest
// already contains the namespace object. But we can not guarantee this because
//  1. The current upstream OSS ArgoCD does not contain "argocd" namespace object.
//  2. We don't have a developer guidance yet nor any validation checks to make sure the Google managed ArgoCD manifests
//     have this namespace object.
func (r *ArgoCDReconciler) namespaceExist(ctx context.Context, objects *manifest.Objects) error {
	log := log.FromContext(ctx)
	for _, object := range objects.Items {
		if object.Kind == "namespace" && object.GetName() == "argocd" {
			return nil
		}
	}
	namespace, err := manifest.ParseJSONToObject([]byte(`
{
  "apiVersion": "v1",
  "kind": "Namespace",
  "metadata": {
	"name": "argocd"
  }
}`))
	if err != nil {
		return errors.Wrap(err, "parse namespace JSON failed")
	}
	objects.Items = append(objects.Items, namespace)
	log.WithValues("namespace", namespace).Info("created namespace")
	return nil
}

// for WithApplyPrune
// +kubebuilder:rbac:groups=*,resources=*,verbs=list

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups=apps;extensions,resources=deployments,verbs=get;list;watch;create;update;delete;patch
