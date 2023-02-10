package argocd

import (
	"context"
	"fmt"

	addonsv1alpha1 "acp.git.corp.google.com/moss/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
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
	// applier := applier.NewApplySetApplier(metav1.PatchOptions{})

	watchLabels := declarative.SourceLabel(mgr.GetScheme())
	if err := r.Reconciler.Init(mgr, &addonsv1alpha1.ArgoCD{},
		declarative.WithObjectTransform(declarative.AddLabels(labels)),
		declarative.WithOwner(declarative.SourceAsOwner),
		declarative.WithObjectTransform(
			declarative.AddLabels(labels),
			r.SetNamespace,
		),
		// TODO: Define `ArgoCD.Status` to ack users the health status; k-d-p side needs to extend the kstatus support.
		declarative.WithStatus(status.NewKstatusCheck(mgr.GetClient(), &r.Reconciler)),
		declarative.WithObjectTransform(addon.ApplyPatches),
		// The default applier relies on kubectl lib to apply and to prune, we should switch to applier.NewApplySetApplier
		// if we want to integrate with GKE HUB.
		// declarative.WithApplier(applier),
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

// SetNamespace guarantees the ArgoCD manifests are in the "argocd" namespace.
// This method is necessary because the upstream OSS argocd manifests do not have "namespace" assigned. Instead,
// it asks users to change the KubeContext to "argocd" namespace for manual installation.
//
// For production reliability concerns, the Google managed ArgoCD manifests should have their namespace assigned
// before checking into the channels/packages using the `set-namespace` KRM function.
//
// This function currently assigns the namespace for the Private Preview ArgoCD manifests. But we cannot guarantee
// the coverage for all the future ArgoCD manifests. Eventually, this method should just be a sanity check,
// and we should rely on kpt or kustomize to assign the right namespace.
func (r *ArgoCDReconciler) SetNamespace(ctx context.Context, _ declarative.DeclarativeObject, objects *manifest.Objects) error {
	log := log.FromContext(ctx)
	for _, object := range objects.Items {
		err := object.SetNestedField(ArgoCDNamespace, "metadata", "namespace")
		if err != nil {
			log.WithValues("kind", object.GroupVersionKind(), "name", object.GetName()).Error(
				err, "unable to set namespace")
			return err
		}
		log.WithValues("object", fmt.Sprintf("%s", object.GroupVersionKind().String())).Info("set namespace to argocd")
	}
	return r.namespaceExist(ctx, objects)
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
