package main

import (
	"context"
	"encoding/json"
	"errors"
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
		return errors.New("missing policy")
	}

	policyFilename := args[0]

	var err error

	var client *tailscale.Client

	if clientID != "" || clientSecret != "" {
		oauthScopes := []string{"devices:read"}
		clientOption := tailscale.WithOAuthClientCredentials(clientID, clientSecret, oauthScopes)
		client, err = tailscale.NewClient(apiKey, "-", clientOption)
	} else if apiKey != "" {
		client, err = tailscale.NewClient(apiKey, "-")
	} else {
		return errors.New("either api key or oauth credentials must be provided")
	}

	if err != nil {
		return fmt.Errorf("failed to create Tailscale client: %w", err)
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
		return nil, fmt.Errorf("failed to fetch Tailscale devices: %w", err)
	}

	hosts := map[string]string{}

	for _, device := range devices {
		name, err := deviceShortDomain(device)
		if err != nil {
			return nil, fmt.Errorf("bad host: %w", err)
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
		return fmt.Errorf("file does not exist: %w", err)
	}

	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed read policy: %w", err)
	}
	defer f.Close()

	src, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("failed read policy: %w", err)
	}

	input := make([]byte, len(src))
	_ = copy(input, src)

	value, err := hujson.Parse(input)
	if err != nil {
		return fmt.Errorf("failed parse policy: %w", err)
	}

	patchOp := JSONPatchOperation{
		Operation: "replace",
		Path:      "/hosts",
		Value:     hosts,
	}
	patch := []JSONPatchOperation{patchOp}

	patchjson, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to update policy: %w", err)
	}

	err = value.Patch(patchjson)
	if err != nil {
		return fmt.Errorf("failed to update policy: %w", err)
	}

	value.Format()

	err = os.WriteFile(filename, []byte(value.String()), info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("failed to write policy: %w", err)
	}

	return nil
}

func deviceShortDomain(device tailscale.Device) (string, error) {
	parts := strings.Split(device.Name, ".")
	if len(parts) < 3 {
		return "", fmt.Errorf("bad device name: %s", device.Name)
	}

	if parts[len(parts)-2] == "ts" && parts[len(parts)-1] == "net" {
		return parts[0], nil
	}

	return "", fmt.Errorf("bad device name: %s", device.Name)
}
