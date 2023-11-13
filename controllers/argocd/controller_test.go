package argocd

import (
	"testing"

	addonsv1alpha1 "github.com/yuwenma/moss/moss/api/v1alpha1"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/test/golden"
)

func TestArgoCDController(t *testing.T) {
	validator := golden.NewValidator(t, addonsv1alpha1.SchemeBuilder)
	dr := &ArgoCDReconciler{
		Client: validator.Client(),
	}
	err := dr.SetupWithManager(validator.Manager())
	if err != nil {
		t.Fatalf("creating reconciler: %v", err)
	}

	validator.Validate(dr.Reconciler)
}
