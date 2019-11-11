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
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	webappv1alpha1 "github.com/scnewma/app-controller/api/v1alpha1"
)

// WebAppReconciler reconciles a WebApp object
type WebAppReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=webapp.com.shaunnewman,resources=webapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=webapp.com.shaunnewman,resources=webapps/status,verbs=get;update;patch

func (r *WebAppReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("webapp", req.NamespacedName)
	log.Info("=== Reconciling Webapp")

	instance := &webappv1alpha1.WebApp{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	deployment := &appsv1.Deployment{}
	err = r.Get(ctx, req.NamespacedName, deployment)
	if errors.IsNotFound(err) {
		deployment := newDeployment(instance)
		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			// requeue with error
			return reconcile.Result{}, err
		}

		err = r.Create(ctx, deployment)
		if err != nil {
			// requeue with error
			return reconcile.Result{}, err
		}
	} else if err != nil {
		// requeue with error
		return reconcile.Result{}, err
	} else if instance.Spec.Instances != *deployment.Spec.Replicas {
		log.V(4).Info(fmt.Sprintf("WebApp %s replicas %d, deployment replicas: %d", req.NamespacedName, instance.Spec.Instances, deployment.Spec.Replicas))
		err := r.Update(ctx, deployment)
		if err != nil {
			// requeue with error
			return reconcile.Result{}, err
		}
	}

	service := &corev1.Service{}
	err = r.Get(ctx, req.NamespacedName, service)
	if errors.IsNotFound(err) {
		service := newService(instance)
		err := controllerutil.SetControllerReference(instance, service, r.Scheme)
		if err != nil {
			// requeue with error
			return reconcile.Result{}, err
		}

		err = r.Create(ctx, service)
		if err != nil {
			// requeue with error
			return reconcile.Result{}, err
		}
	} else if err != nil {
		// requeue with error
		return reconcile.Result{}, err
	}

	ingress := &networkingv1beta1.Ingress{}
	err = r.Get(ctx, req.NamespacedName, ingress)
	if errors.IsNotFound(err) {
		if instance.Spec.NoRoute {
			// nothing else do to
			return reconcile.Result{}, nil
		}

		ingress = newIngress(instance)
		err = controllerutil.SetControllerReference(instance, ingress, r.Scheme)
		if err != nil {
			// requeue with error
			return reconcile.Result{}, err
		}

		err = r.Create(ctx, ingress)
		if err != nil {
			// requeue with error
			return reconcile.Result{}, err
		}
	} else if err != nil {
		// requeue with error
		return reconcile.Result{}, err
	}

	return ctrl.Result{}, nil
}

func newDeployment(instance *webappv1alpha1.WebApp) *appsv1.Deployment {
	labels := map[string]string{
		"app": instance.Name,
	}

	env := []corev1.EnvVar{}
	for k, v := range instance.Spec.Env {
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}

	probe := &corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: instance.Spec.HealthCheckEndpoint,
				Port: intstr.FromInt(8080),
			},
		},
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
	}

	resources := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(instance.Spec.CPU),
		corev1.ResourceMemory: resource.MustParse(instance.Spec.Memory),
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32ptr(instance.Spec.Instances),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:   instance.Name,
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  instance.Name,
							Image: instance.Spec.Image,
							Env:   env,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							LivenessProbe:  probe,
							ReadinessProbe: probe,
							Resources: corev1.ResourceRequirements{
								Limits:   resources,
								Requests: resources,
							},
						},
					},
				},
			},
		},
	}
}

func newService(instance *webappv1alpha1.WebApp) *corev1.Service {
	labels := map[string]string{
		"app": instance.Name,
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
				},
			},
		},
	}
}

func newIngress(instance *webappv1alpha1.WebApp) *networkingv1beta1.Ingress {
	rules := []networkingv1beta1.IngressRule{}
	for _, route := range instance.Spec.Routes {
		rules = append(rules, networkingv1beta1.IngressRule{
			Host: route,
			IngressRuleValue: networkingv1beta1.IngressRuleValue{
				HTTP: &networkingv1beta1.HTTPIngressRuleValue{
					Paths: []networkingv1beta1.HTTPIngressPath{
						{
							Path: "/",
							Backend: networkingv1beta1.IngressBackend{
								ServiceName: instance.Name,
								ServicePort: intstr.FromInt(8080),
							},
						},
					},
				},
			},
		})
	}

	return &networkingv1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: networkingv1beta1.IngressSpec{
			Rules: rules,
		},
	}
}

var (
	apiGVStr = webappv1alpha1.GroupVersion.String()
	ownerKey = ".metadata.controller"
)

func (r *WebAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(&appsv1.Deployment{}, ownerKey, func(rawObj runtime.Object) []string {
		dep := rawObj.(*appsv1.Deployment)
		owner := metav1.GetControllerOf(dep)
		if owner == nil {
			return nil
		}

		if owner.APIVersion != apiGVStr || owner.Kind != "WebApp" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(&corev1.Service{}, ownerKey, func(rawObj runtime.Object) []string {
		svc := rawObj.(*corev1.Service)
		owner := metav1.GetControllerOf(svc)
		if owner == nil {
			return nil
		}

		if owner.APIVersion != apiGVStr || owner.Kind != "WebApp" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(&networkingv1beta1.Ingress{}, ownerKey, func(rawObj runtime.Object) []string {
		ing := rawObj.(*networkingv1beta1.Ingress)
		owner := metav1.GetControllerOf(ing)
		if owner == nil {
			return nil
		}

		if owner.APIVersion != apiGVStr || owner.Kind != "WebApp" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&webappv1alpha1.WebApp{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1beta1.Ingress{}).
		Complete(r)
}

func int32ptr(i int32) *int32 {
	return &i
}
