package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rajeshkio/cf-tunnel-operator/pkg/cloudflare"
)

func main() {
	accountID := os.Getenv("CF_ACCOUNT_ID")
	tunnelID := os.Getenv("CF_TUNNEL_ID")
	apiToken := os.Getenv("CF_API_TOKEN")

	if accountID == "" || tunnelID == "" || apiToken == "" {
		fmt.Println("Error: please set CF_ACCOUNT_ID, CF_TUNNEL_ID, CF_API_TOKEN")
		os.Exit(1)
	}

	client := cloudflare.NewClient(accountID, tunnelID, apiToken)
	ctx := context.Background()

	fmt.Println("Fetching tunnel config")
	config, err := client.GetTunnelConfig(ctx)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	fmt.Println("Got tunnel config")
	fmt.Println("Number of rules ", len(config.Rules))
	fmt.Println("")

	for i, rule := range config.Rules {
		fmt.Printf("Rule %d:\n", i+1)
		fmt.Printf("  Hostname: %s\n", rule.Hostname)
		fmt.Printf("  Service:  %s\n", rule.Service)
		fmt.Println("")
	}
}
