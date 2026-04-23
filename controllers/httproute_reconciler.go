package controllers

import (
	"context"
	"fmt"
	"slices"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	cf "github.com/rajeshkio/cf-tunnel-operator/pkg/cloudflare"
)

const finalizer = "cloudflare-tunnel.rajesh-kumar.in/cleanup"

type HTTPRouteReconciler struct {
	client.Client
	CF *cf.Client
}

func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	fmt.Println("Reconcile called for: ", req.NamespacedName)

	var route gatewayv1.HTTPRoute
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		fmt.Println("HTTPRoute not found, probably deleted:", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	//fmt.Println("Found HTTPRoute:", route.Name, "in namespace", route.Namespace)

	if !route.DeletionTimestamp.IsZero() {
		fmt.Println("HTTPRoute is being deleted:", req.NamespacedName)
		if containsFinalizer(route.Finalizers, finalizer) {
			config, err := r.CF.GetTunnelConfig(ctx)
			if err != nil {
				fmt.Println("Failed to get tunnel config:", err)
				return ctrl.Result{}, err
			}

			hostnameExists := false
			hostname := ""
			if len(route.Spec.Hostnames) > 0 {
				hostname = string(route.Spec.Hostnames[0])
			}

			var newRules []cf.TunnelRule
			for _, rule := range config.Rules {
				if rule.Hostname == hostname {
					hostnameExists = true
				} else if rule.Hostname != "" {
					newRules = append(newRules, rule)
				}
			}
			newRules = append(newRules, cf.TunnelRule{
				Service: "http_status:404",
			})

			if hostnameExists {
				fmt.Println("Removing", hostname, "from Cloudflare tunnel...")
				if err := r.CF.PutTunnelConfig(ctx, cf.TunnelConfig{
					Rules: newRules,
				}); err != nil {
					fmt.Println("Failed to update tunnel config:", err)
					return ctrl.Result{}, err
				}
				fmt.Println("Removed from Cloudflare")
			} else {
				fmt.Println("Hostname already gone from Cloudflare, skipping PUT")
			}

			route.Finalizers = removeFinalizer(route.Finalizers, finalizer)
			if err := r.Update(ctx, &route); err != nil {
				fmt.Println("Failed to remove finalizer:", err)
				return ctrl.Result{}, err
			}
			fmt.Println("Finalizer cleared")
			return ctrl.Result{}, nil
		}

	}

	if !containsFinalizer(route.Finalizers, finalizer) {
		fmt.Println("Adding finalizer to:", req.NamespacedName)
		route.Finalizers = append(route.Finalizers, finalizer)
		if err := r.Update(ctx, &route); err != nil {
			fmt.Println("Failed to add finalizer:", err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	config, err := r.CF.GetTunnelConfig(ctx)
	if err != nil {
		fmt.Println("Failed to get tunnel config", err)
		return ctrl.Result{}, err
	}

	fmt.Println("Current tunnel has", len(config.Rules), "rules")

	if len(route.Spec.Hostnames) == 0 {
		fmt.Println("No hostnames found, skipping")
		return ctrl.Result{}, nil
	}

	if len(route.Spec.Rules) == 0 || len(route.Spec.Rules[0].BackendRefs) == 0 {
		fmt.Println("No backend refs found, skipping")
		return ctrl.Result{}, nil
	}

	hostname := string(route.Spec.Hostnames[0])
	backend := route.Spec.Rules[0].BackendRefs[0]
	service := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", backend.Name, route.Namespace, *backend.Port)

	fmt.Println("Building tunnel rule:")
	fmt.Println("  hostname:", hostname)
	fmt.Println("  service: ", service)

	for _, rule := range config.Rules {
		if rule.Hostname == hostname && rule.Service == service {
			fmt.Println("No changes detected, skipping:", hostname)
			return ctrl.Result{}, nil
		}
	}

	var newRules []cf.TunnelRule
	for _, rule := range config.Rules {
		if rule.Hostname != hostname && rule.Hostname != "" {
			newRules = append(newRules, rule)
		}
	}

	newRules = append(newRules, cf.TunnelRule{
		Hostname: hostname,
		Service:  service,
	})

	newRules = append(newRules, cf.TunnelRule{
		Service: "http_status:404",
	})
	fmt.Println("Pushing", len(newRules), "rules to Cloudflare...")
	err = r.CF.PutTunnelConfig(ctx, cf.TunnelConfig{
		Rules: newRules,
	})
	if err != nil {
		fmt.Println("Failed to update tunnel config:", err)
		return ctrl.Result{}, err
	}
	fmt.Println("Done.")
	return ctrl.Result{}, nil
}

func containsFinalizer(finalizers []string, name string) bool {
	return slices.Contains(finalizers, name)
}

func removeFinalizer(finalizers []string, name string) []string {
	var result []string
	for _, f := range finalizers {
		if f != name {
			result = append(result, f)
		}
	}
	return result
}
func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&gatewayv1.HTTPRoute{}).Complete(r)
}
