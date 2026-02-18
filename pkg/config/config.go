package config

type Config struct {
	OpenFGA struct {
		Addr string `mapstructure:"openfga-addr" default:"openfga.platform-mesh-system:8081"`
	} `mapstructure:",squash"`

	Webhook struct {
		CertDir                    string   `mapstructure:"webhook-cert-dir" default:"config"`
		ClusterKey                 string   `mapstructure:"webhook-cluster-key" default:"authorization.kubernetes.io/cluster-name"`
		AllowedNonResourcePrefixes []string `mapstructure:"webhook-allowed-nonresource-prefixes" default:"/api,/openapi,/version"`
	} `mapstructure:",squash"`

	KCP struct {
		APIExportEndpointSliceName string `mapstructure:"kcp-api-export-endpoint-slice-name" default:"core.platform-mesh.io"`
	} `mapstructure:",squash"`
}
