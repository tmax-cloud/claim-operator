/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/prometheus/common/log"
	claimsv1alpha1 "github.com/tmax-cloud/claim-operator/api/v1alpha1"
	clusterv1alpha1 "github.com/tmax-cloud/cluster-manager-operator/api/v1alpha1"

	// hyperv1 "github.com/tmax-cloud/claim-operator/hyper/v1"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
	// clustersv1alpha1 "github.com/tmax-cloud/cluster-manager-operator/api/v1alpha1"
)

const (
	CLAIM_API_GROUP             = "claims.tmax.io"
	CLAIM_API_Kind              = "clusterclaims"
	CLAIM_API_GROUP_VERSION     = "claims.tmax.io/v1alpha1"
	HCR_API_GROUP_VERSION       = "multi.hyper.tmax.io/v1"
	HYPERCLOUD_SYSTEM_NAMESPACE = "hypercloud5-system"
	HCR_SYSTEM_NAMESPACE        = "kube-federation-system"
)

var AutoAdmit bool

// ClusterClaimReconciler reconciles a ClusterClaim object
type ClusterClaimReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=claims.tmax.io,resources=clusterclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=claims.tmax.io,resources=clusterclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.tmax.io,resources=clustermanagers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.tmax.io,resources=clustermanagers/status,verbs=get;update;patch

func (r *ClusterClaimReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	log := r.Log.WithValues("ClusterClaim", req.NamespacedName)

	clusterClaim := &claimsv1alpha1.ClusterClaim{}

	if err := r.Get(context.TODO(), req.NamespacedName, clusterClaim); err != nil {
		if errors.IsNotFound(err) {
			log.Info("ClusterClaim resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ClusterClaim")
		return ctrl.Result{}, err
	} else if clusterClaim.Status.Phase == "" {
		if err := r.CreateClaimRole(clusterClaim); err != nil {
			return ctrl.Result{}, err
		}
	}

	if AutoAdmit == false {
		if clusterClaim.Status.Phase == "" {
			clusterClaim.Status.Phase = "Awaiting"
			clusterClaim.Status.Reason = "Waiting for admin approval"
			err := r.Status().Update(context.TODO(), clusterClaim)
			if err != nil {
				log.Error(err, "Failed to update ClusterClaim status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		} else if clusterClaim.Status.Phase == "Awaiting" {
			return ctrl.Result{}, nil
		}
	}

	if clusterClaim.Status.Phase == "Rejected" {
		return ctrl.Result{}, nil
	} else if clusterClaim.Status.Phase == "Admitted" {
		if err := r.CreateClusterManager(clusterClaim); err != nil {
			return ctrl.Result{}, err
		}

		clusterClaim.Status.Phase = "Success"
		if err := r.Status().Update(context.TODO(), clusterClaim); err != nil {
			log.Error(err, "Failed to update ClusterClaim status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *ClusterClaimReconciler) CreateClaimRole(clusterClaim *claimsv1alpha1.ClusterClaim) error {
	role := &rbacv1.Role{}
	roleName := clusterClaim.Annotations["creator"] + "-" + clusterClaim.Name + "-cc-role"
	roleKey := types.NamespacedName{Name: roleName, Namespace: HYPERCLOUD_SYSTEM_NAMESPACE}
	if err := r.Get(context.TODO(), roleKey, role); err != nil {
		if errors.IsNotFound(err) {
			newRole := &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roleName,
					Namespace: HYPERCLOUD_SYSTEM_NAMESPACE,
				},
				Rules: []rbacv1.PolicyRule{
					{APIGroups: []string{CLAIM_API_GROUP}, Resources: []string{"clusterclaims"},
						ResourceNames: []string{clusterClaim.Name}, Verbs: []string{"get"}},
					{APIGroups: []string{CLAIM_API_GROUP}, Resources: []string{"clusterclaims/status"},
						ResourceNames: []string{clusterClaim.Name}, Verbs: []string{"get"}},
				},
			}
			ctrl.SetControllerReference(clusterClaim, newRole, r.Scheme)
			err = r.Create(context.TODO(), newRole)
			if err != nil {
				log.Error(err, "Failed to create "+roleName+" role.")
				return err
			}
		} else {
			log.Error(err, "Failed to get role")
			return err
		}
	}

	roleBinding := &rbacv1.RoleBinding{}
	roleBindingName := clusterClaim.Annotations["creator"] + "-" + clusterClaim.Name + "-cc-rolebinding"
	roleBindingKey := types.NamespacedName{Name: clusterClaim.Name, Namespace: HYPERCLOUD_SYSTEM_NAMESPACE}
	if err := r.Get(context.TODO(), roleBindingKey, roleBinding); err != nil {
		if errors.IsNotFound(err) {
			newRoleBinding := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roleBindingName,
					Namespace: HYPERCLOUD_SYSTEM_NAMESPACE,
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "Role",
					Name:     roleName,
				},
				Subjects: []rbacv1.Subject{
					{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "User",
						Name:     clusterClaim.Annotations["creator"],
						// Namespace: HYPERCLOUD_SYSTEM_NAMESPACE,
					},
				},
			}
			ctrl.SetControllerReference(clusterClaim, newRoleBinding, r.Scheme)
			err = r.Create(context.TODO(), newRoleBinding)
			if err != nil {
				log.Error(err, "Failed to create "+roleBindingName+" role.")
				return err
			}
		} else {
			log.Error(err, "Failed to get rolebinding")
			return err
		}
	}
	return nil
}

func (r *ClusterClaimReconciler) CreateClusterManager(clusterClaim *claimsv1alpha1.ClusterClaim) error {
	clm := &clusterv1alpha1.ClusterManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterClaim.Name,
			Namespace: HYPERCLOUD_SYSTEM_NAMESPACE,
			Annotations: map[string]string{
				"owner": clusterClaim.Annotations["creator"],
			},
		},
		FakeObjectMeta: clusterv1alpha1.FakeObjectMeta{
			FakeName: clusterClaim.Spec.ClusterName,
		},
		Spec: clusterv1alpha1.ClusterManagerSpec{
			Provider:   clusterClaim.Spec.Provider,
			Version:    clusterClaim.Spec.Version,
			Region:     clusterClaim.Spec.Region,
			SshKey:     clusterClaim.Spec.SshKey,
			MasterNum:  clusterClaim.Spec.MasterNum,
			MasterType: clusterClaim.Spec.MasterType,
			WorkerNum:  clusterClaim.Spec.WorkerNum,
			WorkerType: clusterClaim.Spec.WorkerType,
		},
		Status: clusterv1alpha1.ClusterManagerStatus{
			Owner: clusterClaim.Annotations["creator"],
		},
	}
	err := r.Create(context.TODO(), clm)
	if err != nil {
		log.Error(err, "Failed to create ClusterManager")
		return err
	}

	err = r.Status().Update(context.TODO(), clm)
	if err != nil {
		log.Error(err, "Failed to update owner in ClusterManager")
		return err
	}

	return nil
}

//

func (r *ClusterClaimReconciler) requeueClusterClaimsForClusterManager(o handler.MapObject) []ctrl.Request {
	clm := o.Object.(*clusterv1alpha1.ClusterManager)
	log := r.Log.WithValues("objectMapper", "clusterManagerToClusterClaim", "namespace", clm.Namespace, "clusterManager", clm.Name)

	// Don't handle deleted clusterManager
	// if !clm.ObjectMeta.DeletionTimestamp.IsZero() {
	// 	log.V(4).Info("clusterManager has a deletion timestamp, skipping mapping.")
	// 	return nil
	// }

	//get clusterManager
	cc := &claimsv1alpha1.ClusterClaim{}
	key := types.NamespacedName{Namespace: HYPERCLOUD_SYSTEM_NAMESPACE, Name: clm.Name}
	if err := r.Get(context.TODO(), key, cc); err != nil {
		if errors.IsNotFound(err) {
			log.Info("ClusterClaim resource not found. Ignoring since object must be deleted.")
			return nil
		}
		log.Error(err, "Failed to get ClusterClaim")
		return nil
	}

	cc.Status.Phase = "Deleted"
	cc.Status.Reason = "cluster is deleted"
	err := r.Status().Update(context.TODO(), cc)
	if err != nil {
		log.Error(err, "Failed to update ClusterClaim status")
		return nil //??
	}
	return nil
}

func (r *ClusterClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	controller, err := ctrl.NewControllerManagedBy(mgr).
		For(&claimsv1alpha1.ClusterClaim{}).
		WithEventFilter(
			predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {

					return true
				},
				UpdateFunc: func(e event.UpdateEvent) bool {

					return true
				},
				DeleteFunc: func(e event.DeleteEvent) bool {

					return false
				},
				GenericFunc: func(e event.GenericEvent) bool {

					return false
				},
			},
		).
		Build(r)

	if err != nil {
		return err
	}

	return controller.Watch(
		&source.Kind{Type: &clusterv1alpha1.ClusterManager{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(r.requeueClusterClaimsForClusterManager),
		},
		predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				return false
			},
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return true
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		},
	)
}
