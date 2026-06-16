package controllers

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/rajeshkio/cf-tunnel-operator/api/v1alpha1"
	cf "github.com/rajeshkio/cf-tunnel-operator/pkg/cloudflare"
)

const finalizer = "cloudflare-tunnel.rajesh-kumar.in/cleanup"

type HTTPRouteReconciler struct {
	client.Client
	CF                *cf.Client
	OperatorNamespace string
}

func handleRateLimit(err error) (ctrl.Result, bool) {
	if errors.Is(err, cf.ErrRateLimited) {
		return ctrl.Result{RequeueAfter: 20 * time.Second}, true
	}
	return ctrl.Result{}, false
}

// Reconcile handles HTTPRoute create, update, and delete events.
func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconcile called for", "route", req.NamespacedName)

	var route gatewayv1.HTTPRoute
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		log.Info("HTTPRoute not found, probably deleted", "route", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the route is being deleted and handle cleanup via finalizer
	if !route.DeletionTimestamp.IsZero() {
		log.Info("HTTPRoute is being deleted", "route", req.NamespacedName)
		if containsFinalizer(route.Finalizers, finalizer) {
			config, err := r.CF.GetTunnelConfig(ctx)
			if err != nil {
				if result, ok := handleRateLimit(err); ok {
					return result, nil
				}
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
					if result, ok := handleRateLimit(err); ok {
						return result, nil
					}
					log.Error(err, "Failed to update tunnel config", "route", req.NamespacedName)
					return ctrl.Result{}, err
				}
				log.Info("Tunnel rule removed", "hostname", hostname)
			} else {
				log.Info("Tunnel rule already gone", "hostname", hostname)
			}

			dnsRecord, err := r.CF.ListDNSRecords(ctx, hostname)
			if err != nil {
				if result, ok := handleRateLimit(err); ok {
					return result, nil
				}
				log.Error(err, "Failed to check DNS record", "hostname", hostname)
				return ctrl.Result{Requeue: true}, nil
			}
			if dnsRecord != nil {
				if err := r.CF.DeleteDNSRecord(ctx, hostname); err != nil {
					if result, ok := handleRateLimit(err); ok {
						return result, nil
					}
					log.Error(err, "Failed to delete DNS record", "hostname", hostname)
					return ctrl.Result{Requeue: true}, nil
				}
				log.Info("DNS record deleted", "hostname", hostname)
			} else {
				log.Info("DNS record already gone, skipping", "hostname", hostname)
			}

			if route.Annotations["cf-tunnel-operator/zero-trust"] == "true" {
				if err := r.CF.DeleteAccessApplication(ctx, hostname); err != nil {
					if result, ok := handleRateLimit(err); ok {
						return result, nil
					}
					log.Error(err, "Failed to delete access application", "hostname", hostname)
					return ctrl.Result{Requeue: true}, nil
				}
				log.Info("Access Application Deleted", "hostname", hostname)
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
		if result, ok := handleRateLimit(err); ok {
			return result, nil
		}
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
	backendAnnotationScheme := "http"
	if route.Annotations["cf-tunnel-operator/backend-scheme"] != "" {
		backendAnnotationScheme = route.Annotations["cf-tunnel-operator/backend-scheme"]
	}
	backendAnnotationTLS := false
	if route.Annotations["cf-tunnel-operator/no-tls-verify"] != "" {
		backendAnnotationTLS, err = strconv.ParseBool(route.Annotations["cf-tunnel-operator/no-tls-verify"])
		if err != nil {
			log.Error(err, "failed to convert", route.Annotations["cf-tunnel-operator/no-tls-verify"])
		}
	}
	service := fmt.Sprintf("%s://%s.%s.svc.cluster.local:%d", backendAnnotationScheme, backend.Name, route.Namespace, *backend.Port)

	log.Info("Building tunnel rule", "hostname", hostname, "service", service)

	// Check if tunnel rule already matches desired state
	tunnelUpToDate := false
	for _, rule := range config.Rules {
		if rule.Hostname == hostname && rule.Service == service {
			tunnelUpToDate = true
			break
		}
	}

	// Check if DNS CNAME record already exists for this hostname
	dnsRecord, err := r.CF.ListDNSRecords(ctx, hostname)
	if err != nil {
		if result, ok := handleRateLimit(err); ok {
			return result, nil
		}
		log.Error(err, "Failed to check DNS record", "hostname", hostname)
		return ctrl.Result{Requeue: true}, nil
	}

	dnsUpToDate := dnsRecord != nil

	// Both tunnel and DNS are in sync, nothing to do
	if dnsUpToDate && tunnelUpToDate {
		appId, policyId, err := r.ensureZeroTrust(ctx, route, hostname)
		if err != nil {
			if result, ok := handleRateLimit(err); ok {
				return result, nil
			}
			log.Error(err, "failed to add", err)
			return ctrl.Result{}, err
		}
		log.Info("No changes detected, skipping", "hostname", hostname)
		err = r.upsertTunnelStatus(ctx, route, v1alpha1.TunnelStatusStatus{
			Hostname:       hostname,
			BackendService: service,
			NoTLSVerify:    backendAnnotationTLS,
			Scheme:         backendAnnotationScheme,
			LastSyncTime:   metav1.Now(),
			SyncStatus:     "Success",
			Message:        "",
			AccessAppID:    appId,
			AccessPolicyID: policyId,
		})
		if err != nil {
			log.Error(err, "Failed to upsert TunnelStatus", "route", req.NamespacedName)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Tunnel rule is missing or outdated, rebuild and push
	if !tunnelUpToDate {
		var newRules []cf.TunnelRule
		for _, rule := range config.Rules {
			if rule.Hostname != hostname && rule.Hostname != "" {
				newRules = append(newRules, rule)
			}
		}

		newRules = append(newRules, cf.TunnelRule{
			Hostname: hostname,
			Service:  service,
			OriginRequest: cf.OriginRequest{
				NoTLSVerify: backendAnnotationTLS,
			},
		})

		newRules = append(newRules, cf.TunnelRule{
			Service: "http_status:404",
		})
		log.Info("Pushing rules to Cloudflare", "count", len(newRules))
		err = r.CF.PutTunnelConfig(ctx, cf.TunnelConfig{
			Rules: newRules,
		})
		if err != nil {
			if result, ok := handleRateLimit(err); ok {
				return result, nil
			}
			log.Error(err, "Failed to update tunnel config", "route", req.NamespacedName)
			if upsertErr := r.upsertTunnelStatus(ctx, route, v1alpha1.TunnelStatusStatus{
				Hostname:       hostname,
				BackendService: service,
				NoTLSVerify:    backendAnnotationTLS,
				Scheme:         backendAnnotationScheme,
				LastSyncTime:   metav1.Now(),
				SyncStatus:     "Failed",
				Message:        err.Error(),
			}); upsertErr != nil {
				log.Error(err, "Failed to upsert TunnelStatus", "route", req.NamespacedName)
			}
			return ctrl.Result{}, err
		}
	}
	// Ensure DNS CNAME record exists, create if missing
	err = r.CF.EnsureDNSRecord(ctx, hostname)
	if err != nil {
		if result, ok := handleRateLimit(err); ok {
			return result, nil
		}
		log.Error(err, "failed to add DNS record", "hostname", hostname)
		if upsertErr := r.upsertTunnelStatus(ctx, route, v1alpha1.TunnelStatusStatus{
			Hostname:       hostname,
			BackendService: service,
			NoTLSVerify:    backendAnnotationTLS,
			Scheme:         backendAnnotationScheme,
			LastSyncTime:   metav1.Now(),
			SyncStatus:     "Failed",
			Message:        err.Error(),
		}); upsertErr != nil {
			log.Error(err, "Failed to upsert TunnelStatus", "route", req.NamespacedName)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	log.Info("DNS record ensured", "hostname", hostname)
	appId, policyId, err := r.ensureZeroTrust(ctx, route, hostname)
	if err != nil {
		if result, ok := handleRateLimit(err); ok {
			return result, nil
		}
		log.Error(err, "failed to add", err)
		return ctrl.Result{}, err
	}

	err = r.upsertTunnelStatus(ctx, route, v1alpha1.TunnelStatusStatus{
		Hostname:       hostname,
		BackendService: service,
		NoTLSVerify:    backendAnnotationTLS,
		Scheme:         backendAnnotationScheme,
		LastSyncTime:   metav1.Now(),
		SyncStatus:     "Success",
		Message:        "",
		AccessAppID:    appId,
		AccessPolicyID: policyId,
	})

	if err != nil {
		log.Error(err, "Failed to upsert TunnelStatus", "route", req.NamespacedName)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *HTTPRouteReconciler) ensureZeroTrust(ctx context.Context, route gatewayv1.HTTPRoute, hostname string) (string, string, error) {
	zeroTrustAnnotation := false
	var err error
	log := ctrl.LoggerFrom(ctx)
	if route.Annotations["cf-tunnel-operator/zero-trust"] != "" {
		zeroTrustAnnotation, err = strconv.ParseBool(route.Annotations["cf-tunnel-operator/zero-trust"])
		if err != nil {
			log.Error(err, "failed to convert", route.Annotations["cf-tunnel-operator/zero-trust"])
			return "", "", err
		}
	}
	var zeroTrustEmails []string
	var appId string
	var policyId string
	if zeroTrustAnnotation {
		if route.Annotations["cf-tunnel-operator/zero-trust-emails"] != "" {
			emailsRaw := route.Annotations["cf-tunnel-operator/zero-trust-emails"]
			rawEmails := strings.Split(emailsRaw, ",")
			for _, email := range rawEmails {
				zeroTrustEmails = append(zeroTrustEmails, strings.TrimSpace(email))
			}
		}
		appId, err = r.CF.EnsureAccessApplication(ctx, hostname)
		if err != nil {
			log.Error(err, "failed to create Access application", hostname)
			return "", "", err
		}
		policyId, err = r.CF.EnsureAccessPolicy(ctx, appId, hostname, "allow", zeroTrustEmails)
		if err != nil {
			log.Error(err, "failed to create access policy:", appId)
			return "", "", err
		}
	}
	return appId, policyId, nil
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

func (r *HTTPRouteReconciler) upsertTunnelStatus(ctx context.Context, route gatewayv1.HTTPRoute, tunnelStatusInput v1alpha1.TunnelStatusStatus) error {
	log := ctrl.LoggerFrom(ctx)
	tsResource := &v1alpha1.TunnelStatus{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%s", route.Namespace, route.Name), Namespace: r.OperatorNamespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, tsResource, func() error {
		tsResource.Spec.HTTPRouteName = route.Name
		tsResource.Spec.HTTPRouteNamespace = route.Namespace
		return nil
	})
	if err != nil {
		log.Error(err, "Failed to upsert TunnelStatus", "route")
		return err
	}

	tsResource.Status.Hostname = tunnelStatusInput.Hostname
	tsResource.Status.BackendService = tunnelStatusInput.BackendService
	tsResource.Status.LastSyncTime = tunnelStatusInput.LastSyncTime
	tsResource.Status.NoTLSVerify = tunnelStatusInput.NoTLSVerify
	tsResource.Status.Scheme = tunnelStatusInput.Scheme
	tsResource.Status.SyncStatus = tunnelStatusInput.SyncStatus
	tsResource.Status.Message = tunnelStatusInput.Message
	tsResource.Status.AccessAppID = tunnelStatusInput.AccessAppID
	tsResource.Status.AccessPolicyID = tunnelStatusInput.AccessPolicyID
	if err := r.Status().Update(ctx, tsResource); err != nil {
		log.Error(err, "Failed to update TunnelStatus status")
		return err
	}
	return nil
}
