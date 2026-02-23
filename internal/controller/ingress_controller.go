/*
Copyright 2026.

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
	"fmt"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	i2gw "github.com/kgateway-dev/ingress2gateway/pkg/i2gw"
	emitterir "github.com/kgateway-dev/ingress2gateway/pkg/i2gw/emitter_intermediate"
	"github.com/kgateway-dev/ingress2gateway/pkg/i2gw/emitters/common_emitter"
	"github.com/kgateway-dev/ingress2gateway/pkg/i2gw/emitters/kgateway"
	standard_emitter "github.com/kgateway-dev/ingress2gateway/pkg/i2gw/emitters/standard"
	"github.com/kgateway-dev/ingress2gateway/pkg/i2gw/providers/common"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = i2gw.ProviderName("")

// IngressReconciler reconciles a Ingress object
type IngressReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	DefaultParentGateway string
	IngressClassName     string
}

// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ingress object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/reconcile
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// _ = logf.FromContext(ctx)

	// TODO(user): your logic here
	logger := log.FromContext(ctx)

	// 1. Fetch the Ingress instance
	ingress := &networkingv1.Ingress{}
	err := r.Get(ctx, req.NamespacedName, ingress)
	if err != nil {
		if errors.IsNotFound(err) {
			// Ingress is deleted. Because we use OwnerReferences later,
			// Kubernetes GC will automatically delete the HTTPRoute!
			logger.Info("Ingress resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Ingress")
		return ctrl.Result{}, err
	}

	// 1.1 Filter by Ingress Class
	if !r.shouldProcessIngress(ingress) {
		return ctrl.Result{}, nil
	}

	// 2. Translate Ingress to HTTPRoute using the ingress2gateway logic
	// Note: You will need to adapt this wrapper based on the specific version of
	// the kgateway ingress2gateway fork you import.
	httpRoutes, err := translateIngressToHTTPRoute(ingress)
	if err != nil {
		logger.Error(err, "Failed to translate Ingress to HTTPRoute")
		// Don't requeue if translation fails due to bad data, otherwise it loops forever
		return ctrl.Result{}, nil
	}

	// 3. Apply the generated HTTPRoutes to the cluster
	for _, route := range httpRoutes {

		// Create an empty HTTPRoute object to populate
		desiredRoute := &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      route.Name,
				Namespace: route.Namespace,
			},
		}

		// 4. CreateOrUpdate cleanly handles creating new routes or updating existing ones
		op, err := controllerutil.CreateOrUpdate(ctx, r.Client, desiredRoute, func() error {
			// Set the Ingress as the owner of the HTTPRoute
			if err := controllerutil.SetControllerReference(ingress, desiredRoute, r.Scheme); err != nil {
				return err
			}

			// Copy over the translated specs (Spec, Labels, Annotations)
			desiredRoute.Spec = route.Spec
			desiredRoute.Labels = route.Labels
			desiredRoute.Annotations = route.Annotations

			// 4.1 Update ParentRefs if a parent Gateway is specified
			parentGateway := ingress.Annotations["ingress2gateway.viu.ca/parent-gateway"]
			if parentGateway == "" {
				parentGateway = r.DefaultParentGateway
			}

			if parentGateway != "" {
				gwNamespace := ingress.Namespace
				gwName := parentGateway

				// Check if parentGateway is namespaced (namespace/name)
				if parts := strings.SplitN(parentGateway, "/", 2); len(parts) == 2 {
					gwNamespace = parts[0]
					gwName = parts[1]
				}

				desiredRoute.Spec.ParentRefs = []gatewayv1.ParentReference{
					{
						Namespace: (*gatewayv1.Namespace)(&gwNamespace),
						Name:      gatewayv1.ObjectName(gwName),
					},
				}
			}

			return nil
		})

		if err != nil {
			logger.Error(err, "Failed to CreateOrUpdate HTTPRoute", "HTTPRoute.Namespace", desiredRoute.Namespace, "HTTPRoute.Name", desiredRoute.Name)
			return ctrl.Result{}, err
		}

		logger.Info("Successfully reconciled HTTPRoute", "Operation", op, "HTTPRoute.Name", desiredRoute.Name)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Named("ingress").
		Owns(&gatewayv1.HTTPRoute{}). // Watch for changes to HTTPRoutes we own!
		Complete(r)
}

// shouldProcessIngress returns true if the Ingress should be processed by this controller.
func (r *IngressReconciler) shouldProcessIngress(ingress *networkingv1.Ingress) bool {
	// If no IngressClassName is configured on the reconciler, we process all Ingresses.
	if r.IngressClassName == "" {
		return true
	}

	// 1. Check spec.ingressClassName
	if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName == r.IngressClassName {
		return true
	}

	// 2. Check kubernetes.io/ingress.class annotation (legacy)
	if ingress.Annotations["kubernetes.io/ingress.class"] == r.IngressClassName {
		return true
	}

	return false
}

// Wrapper function to interface with the ingress2gateway library
func translateIngressToHTTPRoute(ing *networkingv1.Ingress) ([]gatewayv1.HTTPRoute, error) {
	// 1. Convert Ingress to ProviderIR
	pIR, errs := common.ToIR([]networkingv1.Ingress{*ing}, make(map[types.NamespacedName]map[string]int32), i2gw.ProviderImplementationSpecificOptions{})
	if len(errs) > 0 {
		return nil, fmt.Errorf("failed to convert Ingress to IR: %v", errs)
	}

	// 2. Convert ProviderIR to EmitterIR
	eIR := emitterir.ToEmitterIR(pIR)

	// 3. Apply common emitter logic (like deduplication)
	commonEmitter := common_emitter.NewEmitter()
	eIR, errs = commonEmitter.Emit(eIR)
	if len(errs) > 0 {
		return nil, fmt.Errorf("failed to apply common emitter logic: %v", errs)
	}

	// 4. Use kGateway emitter or standard emitter to produce GatewayResources
	// We'll try the kGateway emitter primarily.
	var resources i2gw.GatewayResources
	kgwEmitter := kgateway.NewEmitter(&i2gw.EmitterConf{})
	resources, errs = kgwEmitter.Emit(eIR)
	if len(errs) > 0 {
		// Fallback to standard emitter if kGateway fails or for broader compatibility
		stdEmitter := standard_emitter.NewEmitter(&i2gw.EmitterConf{})
		resources, errs = stdEmitter.Emit(eIR)
		if len(errs) > 0 {
			return nil, fmt.Errorf("failed to emit GatewayResources: %v", errs)
		}
	}

	// 5. Extract HTTPRoutes
	var routes []gatewayv1.HTTPRoute
	for _, route := range resources.HTTPRoutes {
		routes = append(routes, route)
	}

	return routes, nil
}
