package main

import (
	"fmt"
	"os"

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

	ctrl.SetLogger(zap.New())

	accountID := os.Getenv("CF_ACCOUNT_ID")
	tunnelID := os.Getenv("CF_TUNNEL_ID")
	apiToken := os.Getenv("CF_API_TOKEN")

	if accountID == "" || tunnelID == "" || apiToken == "" {
		fmt.Println("Error: please set CF_ACCOUNT_ID, CF_TUNNEL_ID, CF_API_TOKEN")
		os.Exit(1)
	}

	cfClient := cf.NewClient(accountID, tunnelID, apiToken)
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		fmt.Println("Failed to create manager:", err)
		os.Exit(1)
	}

	reconciler := &controllers.HTTPRouteReconciler{
		Client: mgr.GetClient(),
		CF:     cfClient,
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		fmt.Println("Failed to setup reconciler: ", err)
		os.Exit(1)
	}

	fmt.Println("Starting operator...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Println("Manager failed", err)
		os.Exit(1)
	}
}
