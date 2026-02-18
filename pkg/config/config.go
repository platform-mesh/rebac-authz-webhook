package config

import (
	"github.com/spf13/pflag"
)

type WebhookConfig struct {
	CertDir                    string
	ClusterKey                 string
	AllowedNonResourcePrefixes []string
}

type Config struct {
	MetricsBindAddress     string
	HealthProbeBindAddress string
	OpenFGAAddr            string

	Webhook WebhookConfig

	APIExportEndpointSliceName string
}

func New() *Config {
	return &Config{
		MetricsBindAddress:     ":9090",
		HealthProbeBindAddress: ":8090",
		OpenFGAAddr:            "openfga.platform-mesh-system:8081",
		Webhook: WebhookConfig{
			CertDir:                    "config",
			ClusterKey:                 "authorization.kubernetes.io/cluster-name",
			AllowedNonResourcePrefixes: []string{"/api", "/openapi", "/version"},
		},

		APIExportEndpointSliceName: "core.platform-mesh.io",
	}
}

func (cfg *Config) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.MetricsBindAddress, "metrics-bind-address", cfg.MetricsBindAddress, "Set the metrics bind address")
	fs.StringVar(&cfg.HealthProbeBindAddress, "health-probe-bind-address", cfg.HealthProbeBindAddress, "Set the health probe bind address")
	fs.StringVar(&cfg.OpenFGAAddr, "openfga-addr", cfg.OpenFGAAddr, "Set the OpenFGA address")
	fs.StringVar(&cfg.Webhook.CertDir, "webhook-cert-dir", cfg.Webhook.CertDir, "Set the webhook certificate directory")
	fs.StringVar(&cfg.Webhook.ClusterKey, "webhook-cluster-key", cfg.Webhook.ClusterKey, "Set the webhook cluster key")
	fs.StringSliceVar(&cfg.Webhook.AllowedNonResourcePrefixes, "webhook-allowed-nonresource-prefixes", cfg.Webhook.AllowedNonResourcePrefixes, "Set the allowed non-resource prefixes for the webhook")
	fs.StringVar(&cfg.APIExportEndpointSliceName, "kcp-api-export-endpoint-slice-name", cfg.APIExportEndpointSliceName, "Set the KCP API export endpoint slice name")
}
