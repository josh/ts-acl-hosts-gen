package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tailscale/hujson"
	"github.com/tailscale/tailscale-client-go/tailscale"
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: ts-acl-hosts-gen [flags] policy.hujson\n")
	flag.PrintDefaults()
}

func main() {
	err := mainE()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		usage()
		os.Exit(1)
	}
}

func mainE() error {
	var apiKey string
	flag.StringVar(&apiKey, "api-key", "", "Tailscale API key")
	if apiKey == "" {
		apiKey = os.Getenv("TS_API_KEY")
	}

	var clientID string
	flag.StringVar(&clientID, "oauth-id", "", "Tailscale OAuth client ID")
	if clientID == "" {
		clientID = os.Getenv("TS_OAUTH_ID")
	}

	var clientSecret string
	flag.StringVar(&clientSecret, "oauth-secret", "", "Tailscale OAuth client secret")
	if clientSecret == "" {
		clientSecret = os.Getenv("TS_OAUTH_SECRET")
	}

	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		return fmt.Errorf("missing policy")
	}
	policyFilename := args[0]

	var clientOption tailscale.ClientOption
	if clientID != "" || clientSecret != "" {
		oauthScopes := []string{"devices:read"}
		clientOption = tailscale.WithOAuthClientCredentials(clientID, clientSecret, oauthScopes)
	} else if apiKey == "" {
		return fmt.Errorf("either api key or oauth credentials must be provided")
	}

	client, err := tailscale.NewClient(apiKey, "-", clientOption)
	if err != nil {
		return fmt.Errorf("failed to create Tailscale client: %v", err)
	}

	fmt.Println("Fetching hosts...")

	hosts, err := fetchHosts(client)
	if err != nil {
		return err
	}

	fmt.Println("Formatting policy...")
	err = patchPolicy(policyFilename, hosts)
	return err
}

func fetchHosts(client *tailscale.Client) (map[string]string, error) {
	devices, err := client.Devices(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Tailscale devices: %v", err)
	}

	hosts := map[string]string{}
	for _, device := range devices {
		name, err := deviceShortDomain(device)
		if err != nil {
			return nil, fmt.Errorf("bad host: %v", err)
		}
		hosts[name] = device.Addresses[0]
	}

	return hosts, nil
}

type JSONPatchOperation struct {
	Operation string      `json:"op"`
	Path      string      `json:"path"`
	Value     interface{} `json:"value"`
}

func patchPolicy(filename string, hosts map[string]string) error {
	info, err := os.Stat(filename)
	if err != nil {
		return fmt.Errorf("file does not exist: %v", err)
	}

	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed read policy: %v", err)
	}
	defer f.Close()

	src, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("failed read policy: %v", err)
	}

	input := make([]byte, len(src))
	_ = copy(input, src)

	value, err := hujson.Parse(input)
	if err != nil {
		return fmt.Errorf("failed parse policy: %v", err)
	}

	patchOp := JSONPatchOperation{
		Operation: "replace",
		Path:      "/hosts",
		Value:     hosts,
	}
	patch := []JSONPatchOperation{patchOp}
	patchjson, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to update policy: %v", err)
	}

	err = value.Patch(patchjson)
	if err != nil {
		return fmt.Errorf("failed to update policy: %v", err)
	}
	value.Format()

	err = os.WriteFile(filename, []byte(value.String()), info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("failed to write policy: %v", err)
	}

	return nil
}

func deviceShortDomain(device tailscale.Device) (string, error) {
	parts := strings.Split(device.Name, ".")
	if len(parts) == 4 && strings.HasPrefix(parts[1], "tail") && parts[2] == "ts" && parts[3] == "net" {
		return parts[0], nil
	}
	return "", fmt.Errorf("bad device name: %s", device.Name)
}
