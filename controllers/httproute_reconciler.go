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
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconcile called for", "route", req.NamespacedName)

	var route gatewayv1.HTTPRoute
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		log.Info("HTTPRoute not found, probably deleted", "route", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !route.DeletionTimestamp.IsZero() {
		log.Info("HTTPRoute is being deleted", "route", req.NamespacedName)
		if containsFinalizer(route.Finalizers, finalizer) {
			config, err := r.CF.GetTunnelConfig(ctx)
			if err != nil {
				log.Error(err, "Failed to get tunnel config", "route", req.NamespacedName)
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
				log.Info("Removing hostname from Cloudflare tunnel", "hostname", hostname)
				if err := r.CF.PutTunnelConfig(ctx, cf.TunnelConfig{Rules: newRules}); err != nil {
					log.Error(err, "Failed to update tunnel config", "route", req.NamespacedName)
					return ctrl.Result{}, err
				}
				log.Info("Tunnel rule removed", "hostname", hostname)
			} else {
				log.Info("Tunnel rule already gone", "hostname", hostname)
			}

			dnsRecord, err := r.CF.ListDNSRecords(ctx, hostname)
			if err != nil {
				log.Error(err, "Failed to check DNS record", "hostname", hostname)
				return ctrl.Result{Requeue: true}, nil
			}
			if dnsRecord != nil {
				if err := r.CF.DeleteDNSRecord(ctx, hostname); err != nil {
					log.Error(err, "Failed to delete DNS record", "hostname", hostname)
					return ctrl.Result{Requeue: true}, nil
				}
				log.Info("DNS record deleted", "hostname", hostname)
			} else {
				log.Info("DNS record already gone, skipping", "hostname", hostname)
			}

			route.Finalizers = removeFinalizer(route.Finalizers, finalizer)
			if err := r.Update(ctx, &route); err != nil {
				log.Error(err, "Failed to remove finalizer", "route", req.NamespacedName)
				return ctrl.Result{}, err
			}
			log.Info("Finalizer cleared")
			return ctrl.Result{}, nil
		}

	}

	if !containsFinalizer(route.Finalizers, finalizer) {
		log.Info("Adding finalizer", "route", req.NamespacedName)
		route.Finalizers = append(route.Finalizers, finalizer)
		if err := r.Update(ctx, &route); err != nil {
			log.Error(err, "Failed to add finalizer", "route", req.NamespacedName)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	config, err := r.CF.GetTunnelConfig(ctx)
	if err != nil {
		log.Error(err, "Failed to update tunnel config", "route", req.NamespacedName)
		return ctrl.Result{}, err
	}

	log.Info("Current tunnel rules", "count", len(config.Rules))

	if len(route.Spec.Hostnames) == 0 {
		log.Info("No hostnames found, skipping")
		return ctrl.Result{}, nil
	}

	if len(route.Spec.Rules) == 0 || len(route.Spec.Rules[0].BackendRefs) == 0 {
		log.Info("No backend refs found, skipping")
		return ctrl.Result{}, nil
	}

	hostname := string(route.Spec.Hostnames[0])
	backend := route.Spec.Rules[0].BackendRefs[0]
	service := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", backend.Name, route.Namespace, *backend.Port)

	log.Info("Building tunnel rule", "hostname", hostname, "service", service)

	for _, rule := range config.Rules {
		if rule.Hostname == hostname && rule.Service == service {
			log.Info("No changes detected, skipping", "hostname", hostname)
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
	log.Info("Pushing rules to Cloudflare", "count", len(newRules))
	err = r.CF.PutTunnelConfig(ctx, cf.TunnelConfig{
		Rules: newRules,
	})
	if err != nil {
		log.Error(err, "Failed to update tunnel config", "route", req.NamespacedName)
		return ctrl.Result{}, err
	}
	err = r.CF.EnsureDNSRecord(ctx, hostname)
	if err != nil {
		log.Error(err, "failed to add DNS record", "hostname", hostname)
		return ctrl.Result{Requeue: true}, nil
	}

	log.Info("Done.")
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
