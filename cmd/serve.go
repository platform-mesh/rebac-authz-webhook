package cmd

import (
	"crypto/tls"
	"net/http"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization/union"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/contextual"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/nonresourceattributes"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/orgs"

	"github.com/kcp-dev/multicluster-provider/apiexport"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts the authorization webhook server",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()

		kcpCfg, err := clientcmd.BuildConfigFromFlags("", serverCfg.KCP.KubeconfigPath)
		if err != nil {
			klog.Exit(err, "unable to construct cluster provider")
		}

		kcpCfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
			return otelhttp.NewTransport(rt)
		})

		provider, err := apiexport.New(kcpCfg, apiexport.Options{
			Scheme: scheme,
		})
		if err != nil {
			klog.Exit(err, "unable to construct cluster provider")
		}

		mgr, err := mcmanager.New(kcpCfg, provider, mcmanager.Options{
			Scheme: scheme,
			Logger: klog.NewKlogr(),
			WebhookServer: webhook.NewServer(webhook.Options{
				CertDir: serverCfg.Webhook.CertDir,
			}),
			Metrics: metricsserver.Options{
				BindAddress: defaultCfg.Metrics.BindAddress,
				TLSOpts: []func(*tls.Config){
					func(c *tls.Config) {
						log.Info().Msg("disabling http/2")
						c.NextProtos = []string{"http/1.1"}
					},
				},
			},
			HealthProbeBindAddress: defaultCfg.HealthProbeBindAddress,
		})
		if err != nil {
			klog.Exit(err, "unable to set up overall controller manager")
		}

		conn, err := grpc.NewClient(serverCfg.OpenFGA.Addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		)
		if err != nil {
			klog.Exit(err, "cannot create grpc client to OpenFGA")
		}

		fga := openfgav1.NewOpenFGAServiceClient(conn)

		extraAttrClusterKey := "authorization.kubernetes.io/cluster-name"

		mgr.GetWebhookServer().Register("/authz", authorization.New(
			klog.NewKlogr(),
			union.New(
				nonresourceattributes.New("/api"),
				orgs.New(fga, extraAttrClusterKey, "TODO:", "TODO:"),
				contextual.New(mgr, fga, extraAttrClusterKey),
			),
		))

		if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
			klog.Exit(err, "unable to set up health check")
		}
		if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
			klog.Exit(err, "unable to set up ready check")
		}

		klog.Info("Starting provider")
		go func() {
			if err := provider.Run(ctx, mgr); err != nil {
				klog.Exit(err, "unable to run provider")
			}
		}()

		klog.Info("starting manager")
		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			klog.Exit(err, "problem running manager")
		}
	},
}
