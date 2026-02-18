package cmd

import (
	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/klog/v2"

	kcpsdkapisv1alpha1 "github.com/kcp-dev/sdk/apis/apis/v1alpha1"

	_ "k8s.io/component-base/logs/json/register"
)

var (
	logOpts = logs.NewOptions()

	rootCmd = &cobra.Command{
		Use: "rebac-authz-webhook",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return logsapi.ValidateAndApply(logOpts, nil)
		},
	}

	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(kcpsdkapisv1alpha1.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	rootCmd.AddCommand(NewServeCmd())

	logsapi.AddFlags(logOpts, rootCmd.PersistentFlags())

}

func Execute() {
	defer klog.Flush()
	cobra.CheckErr(rootCmd.Execute())
}
