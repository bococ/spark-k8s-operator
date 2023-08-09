/*
Copyright 2023 zncdata-labs.

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

package controller

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"

	stackv1alpha1 "github.com/zncdata-labs/spark-history-operator/api/v1alpha1"
)

// SparkHistoryReconciler reconciles a SparkHistory object
type SparkHistoryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=stack.zncdata.net,resources=sparkhistories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=stack.zncdata.net,resources=sparkhistories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=stack.zncdata.net,resources=sparkhistories/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=extensions,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *SparkHistoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("Reconciling SparkHistory")

	sparkHistory := &stackv1alpha1.SparkHistory{}
	if err := r.Get(ctx, req.NamespacedName, sparkHistory); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "unable to fetch SparkHistory")
			return ctrl.Result{}, err
		}
		logger.Info("SparkHistory deleted")
		return ctrl.Result{}, nil
	}

	logger.Info("SparkHistory found", "Name", sparkHistory.Name)

	if len(sparkHistory.Status.Conditions) == 0 {
		sparkHistory.Status.Nodes = []string{}
		sparkHistory.Status.Conditions = append(sparkHistory.Status.Conditions, corev1.ComponentCondition{
			Type:   corev1.ComponentHealthy,
			Status: corev1.ConditionFalse,
		})
		err := r.Status().Update(ctx, sparkHistory)
		if err != nil {
			logger.Error(err, "unable to update SparkHistoryServer status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	} else if sparkHistory.Status.Conditions[0].Status == corev1.ConditionTrue {
		sparkHistory.Status.Conditions[0].Status = corev1.ConditionFalse
		err := r.Status().Update(ctx, sparkHistory)
		if err != nil {
			logger.Error(err, "unable to update SparkHistoryServer status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcilePVC(ctx, sparkHistory); err != nil {
		logger.Error(err, "unable to reconcile PVC")
		return ctrl.Result{}, err
	}

	if err := r.reconcileDeployment(ctx, sparkHistory); err != nil {
		logger.Error(err, "unable to reconcile Deployment")
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, sparkHistory); err != nil {
		logger.Error(err, "unable to reconcile Service")
		return ctrl.Result{}, err
	}

	if err := r.reconcileIngress(ctx, sparkHistory); err != nil {
		logger.Error(err, "unable to reconcile Ingress")
		return ctrl.Result{}, err
	}

	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, &client.ListOptions{Namespace: sparkHistory.Namespace, LabelSelector: labels.SelectorFromSet(sparkHistory.GetLabels())}); err != nil {
		logger.Error(err, "unable to list pods")
		return ctrl.Result{}, err
	}

	podNames := getPodNames(podList.Items)

	if !reflect.DeepEqual(podNames, sparkHistory.Status.Nodes) {
		logger.Info("Updating SparkHistoryServer status", "nodes", podNames)
		sparkHistory.Status.Nodes = podNames
		sparkHistory.Status.Conditions[0].Status = corev1.ConditionTrue
		err := r.Status().Update(ctx, sparkHistory)
		if err != nil {
			logger.Error(err, "unable to update SparkHistoryServer status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil

}

func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

// SetupWithManager sets up the controller with the Manager.
func (r *SparkHistoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&stackv1alpha1.SparkHistory{}).
		Complete(r)
}
