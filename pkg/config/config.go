package config

type Config struct {
	OpenFGA struct {
		Addr string `mapstructure:"openfga-addr" default:"openfga.platform-mesh-system:8081"`
	} `mapstructure:",squash"`

	Webhook struct {
		CertDir string `mapstructure:"webhook-cert-dir" default:"config"`
	} `mapstructure:",squash"`

	KCP struct {
		KubeconfigPath string `mapstructure:"kcp-kubeconfig-path" default:""`
	} `mapstructure:",squash"`
}
