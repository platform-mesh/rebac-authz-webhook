package config

import (
	"strings"
	"time"

	"github.com/spf13/pflag"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
)

const (
	CoreProviderName   = "core"
	SystemProviderName = "system"
	providerSeparator  = "#"
)

type WebhookConfig struct {
	CertDir                    string
	ClusterKey                 string
	AllowedNonResourcePrefixes []string

	// CacheMissMaxRetries is the maximum number of retries per key before stopping.
	CacheMissMaxRetries uint
	// CacheMissTTL is the duration after which retry count resets for a key.
	CacheMissTTL time.Duration
	// CacheMissCleanupInterval is the interval at which keys are checked for expiration.
	CacheMissCleanupInterval time.Duration
	// CacheMissRetryAfter is the delay before retrying on cache miss.
	CacheMissRetryAfter time.Duration
}

type APIExportEndpointSlices struct {
	CorePlatformMeshIO   string
	SystemPlatformMeshIO string
}

type Config struct {
	MetricsBindAddress     string
	HealthProbeBindAddress string
	OpenFGAAddr            string

	Webhook WebhookConfig

	APIExportEndpointSlices APIExportEndpointSlices
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
			CacheMissMaxRetries:        1,
			CacheMissTTL:               5 * time.Minute,
			CacheMissCleanupInterval:   2 * time.Minute,
			CacheMissRetryAfter:        1 * time.Second,
		},
		APIExportEndpointSlices: APIExportEndpointSlices{
			CorePlatformMeshIO:   "core.platform-mesh.io",
			SystemPlatformMeshIO: "system.platform-mesh.io",
		},
	}
}

func (cfg *Config) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.MetricsBindAddress, "metrics-bind-address", cfg.MetricsBindAddress, "Set the metrics bind address")
	fs.StringVar(&cfg.HealthProbeBindAddress, "health-probe-bind-address", cfg.HealthProbeBindAddress, "Set the health probe bind address")
	fs.StringVar(&cfg.OpenFGAAddr, "openfga-addr", cfg.OpenFGAAddr, "Set the OpenFGA address")
	fs.StringVar(&cfg.Webhook.CertDir, "webhook-cert-dir", cfg.Webhook.CertDir, "Set the webhook certificate directory")
	fs.StringVar(&cfg.Webhook.ClusterKey, "webhook-cluster-key", cfg.Webhook.ClusterKey, "Set the webhook cluster key")
	fs.StringSliceVar(&cfg.Webhook.AllowedNonResourcePrefixes, "webhook-allowed-nonresource-prefixes", cfg.Webhook.AllowedNonResourcePrefixes, "Set the allowed non-resource prefixes for the webhook")
	fs.UintVar(&cfg.Webhook.CacheMissMaxRetries, "webhook-cache-miss-max-retries", cfg.Webhook.CacheMissMaxRetries, "Maximum number of retries per cluster on cache miss")
	fs.DurationVar(&cfg.Webhook.CacheMissTTL, "webhook-cache-miss-ttl", cfg.Webhook.CacheMissTTL, "Duration after which cache miss count resets for a cluster")
	fs.DurationVar(&cfg.Webhook.CacheMissCleanupInterval, "webhook-cache-miss-cleanup-interval", cfg.Webhook.CacheMissCleanupInterval, "Interval at which cache miss keys are checked for expiration")
	fs.DurationVar(&cfg.Webhook.CacheMissRetryAfter, "webhook-cache-miss-retry-after", cfg.Webhook.CacheMissRetryAfter, "Delay before retrying on cache miss")
	fs.StringVar(&cfg.APIExportEndpointSlices.CorePlatformMeshIO, "core-api-export-endpoint-slice-name", cfg.APIExportEndpointSlices.CorePlatformMeshIO, "Set the core.platform-mesh.io APIExportEndpointSlice name")
	fs.StringVar(&cfg.APIExportEndpointSlices.SystemPlatformMeshIO, "system-api-export-endpoint-slice-name", cfg.APIExportEndpointSlices.SystemPlatformMeshIO, "Set the system.platform-mesh.io APIExportEndpointSlice name")
}

// StripProviderPrefix removes the provider prefix from a cluster name "core#1kar1u6c65ykt4ea" -> "1kar1u6c65ykt4ea".
func StripProviderPrefix(clusterName multicluster.ClusterName) string {
	if _, after, ok := strings.Cut(clusterName.String(), providerSeparator); ok {
		return after
	}
	return clusterName.String()
}

// MultiProviderName returns a cluster name with provider prefix and separator for multi provider.
// The multi.Provider prefixes cluster names as "providerName#clusterName"
func MultiProviderName(providerName, clusterName string) multicluster.ClusterName {
	return multicluster.ClusterName(providerName + providerSeparator + clusterName)
}
