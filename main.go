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
	"tailscale.com/client/tailscale/v2"
)

var (
	ErrInvalidDeviceName = errors.New("invalid device name: missing ts.net suffix")
	ErrMissingPolicy     = errors.New("missing policy")
	ErrNoCredentials     = errors.New("either api key or oauth credentials must be provided")
)

type Config struct {
	APIKey       string
	ClientID     string
	ClientSecret string
	PolicyFile   string
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: ts-acl-hosts-gen [flags] policy.hujson\n")
	flag.PrintDefaults()
}

func main() {
	ctx := context.Background()

	err := mainE(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		usage()
		os.Exit(1)
	}
}

func parseFlags() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.APIKey, "api-key", "", "Tailscale API key")
	flag.StringVar(&cfg.ClientID, "oauth-id", "", "Tailscale OAuth client ID")
	flag.StringVar(&cfg.ClientSecret, "oauth-secret", "", "Tailscale OAuth client secret")

	flag.Usage = usage
	flag.Parse()

	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("TS_API_KEY")
	}

	if cfg.ClientID == "" {
		cfg.ClientID = os.Getenv("TS_OAUTH_ID")
	}

	if cfg.ClientSecret == "" {
		cfg.ClientSecret = os.Getenv("TS_OAUTH_SECRET")
	}

	args := flag.Args()
	if len(args) != 1 {
		return nil, ErrMissingPolicy
	}

	cfg.PolicyFile = args[0]

	return cfg, nil
}

func createTailscaleClient(cfg *Config) (*tailscale.Client, error) {
	switch {
	case cfg.ClientID != "" && cfg.ClientSecret != "":
		oauthScopes := []string{"devices:core:read"}
		client := &tailscale.Client{
			Tailnet: "-",
			HTTP: tailscale.OAuthConfig{
				ClientID:     cfg.ClientID,
				ClientSecret: cfg.ClientSecret,
				Scopes:       oauthScopes,
			}.HTTPClient(),
		}
		return client, nil
	case cfg.APIKey != "":
		client := &tailscale.Client{
			Tailnet: "-",
			APIKey:  cfg.APIKey,
		}
		return client, nil
	default:
		return nil, ErrNoCredentials
	}
}

func mainE(ctx context.Context) error {
	cfg, err := parseFlags()
	if err != nil {
		return err
	}

	client, err := createTailscaleClient(cfg)
	if err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "Fetching hosts...")

	hosts, err := fetchHosts(ctx, client)
	if err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "Formatting policy...")

	return patchPolicy(cfg.PolicyFile, hosts)
}

func fetchHosts(ctx context.Context, client *tailscale.Client) (map[string]string, error) {
	devices, err := client.Devices().List(ctx)
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

func openPolicy(filename string) (*os.File, os.FileInfo, error) {
	info, err := os.Stat(filename)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("failed to check policy file: %w", err)
		}

		f, err := os.Create(filename)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create policy file: %w", err)
		}

		if _, err := f.WriteString("{\n}"); err != nil {
			f.Close()

			return nil, nil, fmt.Errorf("failed to write initial JSON: %w", err)
		}

		f.Close()

		info, err = os.Stat(filename)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to stat new policy file: %w", err)
		}
	}

	policyFile, err := os.Open(filename)
	if err != nil {
		return nil, nil, fmt.Errorf("failed read policy: %w", err)
	}

	return policyFile, info, nil
}

func patchPolicy(filename string, hosts map[string]string) error {
	policyFile, info, err := openPolicy(filename)
	if err != nil {
		return err
	}
	defer policyFile.Close()

	src, err := io.ReadAll(policyFile)
	if err != nil {
		return fmt.Errorf("failed read policy: %w", err)
	}

	input := make([]byte, len(src))
	copy(input, src)

	value, err := hujson.Parse(input)
	if err != nil {
		return fmt.Errorf("failed parse policy: %w", err)
	}

	var operation string
	if value.Find("hosts") == nil {
		operation = "add"
	} else {
		operation = "replace"
	}

	patchOp := JSONPatchOperation{
		Operation: operation,
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
		return "", fmt.Errorf("%w: %q", ErrInvalidDeviceName, device.Name)
	}

	if parts[len(parts)-2] == "ts" && parts[len(parts)-1] == "net" {
		return parts[0], nil
	}

	return "", fmt.Errorf("%w: %q", ErrInvalidDeviceName, device.Name)
}
