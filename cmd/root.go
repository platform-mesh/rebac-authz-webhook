/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	kcpapisv1alpha1 "github.com/kcp-dev/kcp/sdk/apis/apis/v1alpha1"
	accountsv1alpha1 "github.com/platform-mesh/account-operator/api/v1alpha1"
	pmeshconfig "github.com/platform-mesh/golang-commons/config"
	"github.com/platform-mesh/golang-commons/logger"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var (
	rootCmd = &cobra.Command{
		Use: "rebac-authz-webhook",
	}
	v         *viper.Viper
	serverCfg config.Config
	log       *logger.Logger
	scheme    = runtime.NewScheme()
)

// rootCmd represents the base command when called without any subcommands

func init() {
	utilruntime.Must(accountsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kcpapisv1alpha1.AddToScheme(scheme))

	rootCmd.AddCommand(serveCmd)
	cobra.OnInitialize(initConfig, initLog)

	var err error
	v, defaultCfg, err = pmeshconfig.NewDefaultConfig(rootCmd)
	if err != nil {
		panic(err)
	}
	err = pmeshconfig.BindConfigToFlags(v, serveCmd, &serverCfg)
	if err != nil {
		panic(err)
	}
}

func initConfig() {
	// Parse environment variables into the Config struct
	if err := v.Unmarshal(&defaultCfg); err != nil {
		panic(err)
	}

	// Parse environment variables into the Config struct
	if err := v.Unmarshal(&serverCfg); err != nil {
		panic(err)
	}
}

func Execute() { // coverage-ignore
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}

}

func initLog() { // coverage-ignore
	logcfg := logger.DefaultConfig()
	logcfg.Level = defaultCfg.Log.Level
	logcfg.NoJSON = defaultCfg.Log.NoJson

	var err error
	log, err = logger.New(logcfg)
	if err != nil {
		panic(err)
	}
}
