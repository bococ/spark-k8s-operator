package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stackv1alpha1 "github.com/zncdata-labs/spark-history-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
)

func makePVC(ctx context.Context, instance *stackv1alpha1.SparkHistory, schema *runtime.Scheme) *corev1.PersistentVolumeClaim {
	logger := log.FromContext(ctx)
	labels := instance.GetLabels()
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.GetPvcName(),
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: instance.Spec.Persistence.StorageClass,
			AccessModes:      instance.Spec.Persistence.AccessModes,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: instance.Spec.Persistence.GetSize(),
				},
			},
			VolumeMode: instance.Spec.Persistence.VolumeMode,
		},
	}
	err := ctrl.SetControllerReference(instance, pvc, schema)
	if err != nil {
		logger.Error(err, "Failed to set controller reference")
		return nil
	}
	return pvc
}

func (r *SparkHistoryReconciler) reconcilePVC(ctx context.Context, instance *stackv1alpha1.SparkHistory) error {
	logger := log.FromContext(ctx)
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.GetPvcName()}, pvc)
	if err != nil && errors.IsNotFound(err) {
		pvc := makePVC(ctx, instance, r.Scheme)
		logger.Info("Creating a new PVC", "PVC.Namespace", pvc.Namespace, "PVC.Name", pvc.Name)
		err := r.Client.Create(ctx, pvc)
		if err != nil {
			return err
		}
	} else if err != nil {
		logger.Error(err, "Failed to get PVC")
		return err
	}
	return nil
}

func makeIngress(ctx context.Context, instance *stackv1alpha1.SparkHistory, schema *runtime.Scheme) *v1.Ingress {
	logger := log.FromContext(ctx)
	labels := instance.GetLabels()
	ing := &v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: v1.IngressSpec{
			Rules: []v1.IngressRule{
				{
					Host: instance.Spec.Ingress.Host,
					IngressRuleValue: v1.IngressRuleValue{
						HTTP: &v1.HTTPIngressRuleValue{
							Paths: []v1.HTTPIngressPath{
								{
									Path: "/",
									PathType: func() *v1.PathType {
										pt := v1.PathTypePrefix
										return &pt
									}(),
									Backend: v1.IngressBackend{
										Service: &v1.IngressServiceBackend{
											Name: instance.Name,
											Port: v1.ServiceBackendPort{
												Number: instance.Spec.Service.Port,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	err := ctrl.SetControllerReference(instance, ing, schema)
	if err != nil {
		logger.Error(err, "Failed to set controller reference")
		return nil
	}
	return ing
}

func (r *SparkHistoryReconciler) reconcileIngress(ctx context.Context, instance *stackv1alpha1.SparkHistory) error {
	logger := log.FromContext(ctx)
	obj := makeIngress(ctx, instance, r.Scheme)
	if obj == nil {
		return nil
	}

	if err := CreateOrUpdate(ctx, r.Client, obj); err != nil {
		logger.Error(err, "Failed to create or update ingress")
		return err
	}
	return nil
}

func makeService(ctx context.Context, instance *stackv1alpha1.SparkHistory, schema *runtime.Scheme) *corev1.Service {
	labels := instance.GetLabels()
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        instance.Name,
			Namespace:   instance.Namespace,
			Labels:      labels,
			Annotations: instance.Spec.Service.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:     instance.Spec.Service.Port,
					Name:     "http",
					Protocol: "TCP",
				},
			},
			Selector: labels,
			Type:     instance.GetServiceType(),
		},
	}
	err := ctrl.SetControllerReference(instance, svc, schema)
	if err != nil {
		return nil
	}
	return svc
}

func (r *SparkHistoryReconciler) reconcileService(ctx context.Context, instance *stackv1alpha1.SparkHistory) error {
	logger := log.FromContext(ctx)
	obj := makeService(ctx, instance, r.Scheme)
	if obj == nil {
		return nil
	}

	if err := CreateOrUpdate(ctx, r.Client, obj); err != nil {
		logger.Error(err, "Failed to create or update service")
		return err
	}
	return nil
}

func makeDeployment(ctx context.Context, instance *stackv1alpha1.SparkHistory, schema *runtime.Scheme) *appsv1.Deployment {
	labels := instance.GetLabels()
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &instance.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            instance.Name,
							Image:           instance.GetImageTag(),
							ImagePullPolicy: instance.GetImagePullPolicy(),
							Args: []string{
								"/opt/bitnami/spark/sbin/start-history-server.sh",
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    instance.Spec.Resource.Limits["cpu"],
									corev1.ResourceMemory: instance.Spec.Resource.Limits["memory"],
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 18080,
									Name:          "http",
									Protocol:      "TCP",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      instance.GetNameWithSuffix("-data"),
									MountPath: "/tmp/spark-events",
								},
							},
						},
					},
					Tolerations: instance.Spec.Tolerations,
					Volumes: []corev1.Volume{
						{
							Name: instance.GetNameWithSuffix("-data"),
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: instance.GetPvcName(),
								},
							},
						},
					},
				},
			},
		},
	}
	err := ctrl.SetControllerReference(instance, dep, schema)
	if err != nil {
		return nil
	}
	return dep
}

func (r *SparkHistoryReconciler) reconcileDeployment(ctx context.Context, instance *stackv1alpha1.SparkHistory) error {
	logger := log.FromContext(ctx)
	obj := makeDeployment(ctx, instance, r.Scheme)
	if obj == nil {
		return nil
	}
	if err := CreateOrUpdate(ctx, r.Client, obj); err != nil {
		logger.Error(err, "Failed to create or update deployment")
		return err
	}
	return nil
}
