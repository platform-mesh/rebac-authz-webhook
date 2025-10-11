package cmd

import (
	"flag"

	kcpapisv1alpha1 "github.com/kcp-dev/kcp/sdk/apis/apis/v1alpha1"
	tenancyv1alpha1 "github.com/kcp-dev/kcp/sdk/apis/tenancy/v1alpha1"
	accountsv1alpha1 "github.com/platform-mesh/account-operator/api/v1alpha1"
	pmconfig "github.com/platform-mesh/golang-commons/config"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
)

var (
	rootCmd = &cobra.Command{
		Use: "rebac-authz-webhook",
	}

	v          *viper.Viper
	defaultCfg *pmconfig.CommonServiceConfig
	serverCfg  config.Config
	scheme     = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(accountsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(kcpapisv1alpha1.AddToScheme(scheme))
	utilruntime.Must(tenancyv1alpha1.AddToScheme(scheme))

	rootCmd.AddCommand(serveCmd)

	var err error
	v, defaultCfg, err = pmconfig.NewDefaultConfig(rootCmd)
	if err != nil {
		panic(err)
	}

	err = pmconfig.BindConfigToFlags(v, serveCmd, &serverCfg)
	if err != nil {
		panic(err)
	}

	klogFlagSet := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlagSet)

	pflag.CommandLine.AddGoFlagSet(klogFlagSet)
	rootCmd.PersistentFlags().AddGoFlagSet(klogFlagSet)
}

func Execute() {
	defer klog.Flush()
	cobra.CheckErr(rootCmd.Execute())
}
