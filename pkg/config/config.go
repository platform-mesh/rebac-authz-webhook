package config

type Config struct {
	OpenFGA struct {
		Addr string `mapstructure:"openfga-addr" default:"openfga.platform-mesh-system:8081"`
	} `mapstructure:",squash"`

	Webhook struct {
		CertDir    string `mapstructure:"webhook-cert-dir" default:"config"`
		ClusterKey string `mapstructure:"webhook-cluster-key" default:"authorization.kubernetes.io/cluster-name"`
	} `mapstructure:",squash"`

	KCP struct {
		KubeconfigPath string `mapstructure:"kcp-kubeconfig-path" default:""`
	} `mapstructure:",squash"`
}
