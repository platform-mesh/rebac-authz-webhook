package config

type Config struct {
	OpenFGA struct {
		Addr string `mapstructure:"openfga-addr" default:"openfga.openmfp-system:8081"`
	} `mapstructure:",squash"`
	Webhook struct {
		Audience string `mapstructure:"webhook-audience"`
		CertDir  string `mapstructure:"webhook-cert-dir" default:"config"`
	} `mapstructure:",squash"`
	Kcp struct {
		KubeconfigPath  string `mapstructure:"kcp-kubeconfig-path" default:""`
		ClusterURL      string `mapstructure:"kcp-cluster-url" default:""`
		AccountInfoName string `mapstructure:"kcp-account-info-name" default:"account"`
	} `mapstructure:",squash"`
}
