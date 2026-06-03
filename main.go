package main

import (
	"fmt"
	"os"

	"github.com/rajeshkio/cf-tunnel-operator/api/v1alpha1"
	"github.com/rajeshkio/cf-tunnel-operator/controllers"
	cf "github.com/rajeshkio/cf-tunnel-operator/pkg/cloudflare"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func main() {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	gatewayv1.Install(scheme)
	v1alpha1.AddToScheme(scheme)

	ctrl.SetLogger(zap.New())

	accountID := os.Getenv("CF_ACCOUNT_ID")
	tunnelID := os.Getenv("CF_TUNNEL_ID")
	apiToken := os.Getenv("CF_API_TOKEN")
	zoneID := os.Getenv("CF_DNS_ZONE_ID")
	operatorNamespace := os.Getenv("POD_NAMESPACE")

	if accountID == "" || tunnelID == "" || apiToken == "" || zoneID == "" || operatorNamespace == "" {
		fmt.Println("Error: Please set CF_ACCOUNT_ID, CF_TUNNEL_ID, CF_API_TOKEN, CF_DNS_ZONE_ID", "POD_NAMESPACE")
		os.Exit(1)
	}

	cfClient := cf.NewClient(accountID, tunnelID, apiToken, zoneID)
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		fmt.Println("Failed to create manager:", err)
		os.Exit(1)
	}

	reconciler := &controllers.HTTPRouteReconciler{
		Client:            mgr.GetClient(),
		CF:                cfClient,
		OperatorNamespace: operatorNamespace,
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		fmt.Println("Failed to setup reconciler: ", err)
		os.Exit(1)
	}

	fmt.Println("Starting operator...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Println("Manager failed with error:", err)
		os.Exit(1)
	}
}
