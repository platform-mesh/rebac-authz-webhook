package cmd

import (
	"crypto/tls"
	"net/http"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization/union"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/clustercache"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/config"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/contextual"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/nonresourceattributes"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/orgs"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/retry"
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
	multiprovider "sigs.k8s.io/multicluster-runtime/providers/multi"

	"github.com/kcp-dev/multicluster-provider/apiexport"
	pathaware "github.com/kcp-dev/multicluster-provider/path-aware"
)

var serverCfg *config.Config

func NewServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Starts the authorization webhook server",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			ctrl.SetLogger(klog.NewKlogr())

			restCfg := ctrl.GetConfigOrDie()

			restCfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
				return otelhttp.NewTransport(rt)
			})

			systemProvider, err := pathaware.New(restCfg, serverCfg.APIExportEndpointSlices.SystemPlatformMeshIO, apiexport.Options{
				Scheme: scheme,
			})
			if err != nil {
				klog.Exit(err, "unable to create system apiexport provider")
			}
			coreProvider, err := pathaware.New(restCfg, serverCfg.APIExportEndpointSlices.CorePlatformMeshIO, apiexport.Options{
				Scheme: scheme,
			})
			if err != nil {
				klog.Exit(err, "unable to create core apiexport provider")
			}

			multiProv := multiprovider.New(multiprovider.Options{})
			if err := multiProv.AddProvider(config.SystemProviderName, systemProvider); err != nil {
				klog.Exit(err, "unable to add system apiexport provider")
			}
			if err := multiProv.AddProvider(config.CoreProviderName, coreProvider); err != nil {
				klog.Exit(err, "unable to add core apiexport provider")
			}

			// Use Root KCP config for manager
			mgr, err := mcmanager.New(restCfg, multiProv, mcmanager.Options{
				Scheme: scheme,
				Logger: klog.NewKlogr(),
				WebhookServer: webhook.NewServer(webhook.Options{
					CertDir: serverCfg.Webhook.CertDir,
				}),
				Metrics: metricsserver.Options{
					BindAddress: serverCfg.MetricsBindAddress,
					TLSOpts: []func(*tls.Config){
						func(c *tls.Config) {
							klog.Info("disabling http/2")
							c.NextProtos = []string{"http/1.1"}
						},
					},
				},
				HealthProbeBindAddress: serverCfg.HealthProbeBindAddress,
			})
			if err != nil {
				klog.Exit(err, "unable to set up overall controller manager")
			}

			clusterCache, err := clustercache.New(mgr)
			if err != nil {
				klog.Exit(err, "failed to create cluster cache")
			}

			conn, err := grpc.NewClient(serverCfg.OpenFGAAddr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
			)
			if err != nil {
				klog.Exit(err, "cannot create grpc client to OpenFGA")
			}
			defer conn.Close() //nolint:errcheck

			fga := openfgav1.NewOpenFGAServiceClient(conn)

			storeRes, err := fga.ListStores(ctx, &openfgav1.ListStoresRequest{Name: "orgs"})
			if err != nil {
				klog.Exit(err, "cannot list stores from OpenFGA")
			}
			if len(storeRes.Stores) == 0 {
				klog.Exit("no stores found in OpenFGA")
			}
			klog.InfoS("using OpenFGA store", "id", storeRes.Stores[0].Id)

			extraAttrClusterKey := serverCfg.Webhook.ClusterKey
			cacheMissTracker := retry.NewExpiringRetryTracker[string](ctx, serverCfg.Webhook.CacheMissMaxRetries, serverCfg.Webhook.CacheMissTTL)
			mgr.GetWebhookServer().Register("/authz", authorization.New(
				klog.NewKlogr(),
				union.New(
					nonresourceattributes.New(serverCfg.Webhook.AllowedNonResourcePrefixes...),
					orgs.New(fga, mgr, extraAttrClusterKey, storeRes.Stores[0].Id),
					contextual.New(fga, clusterCache, extraAttrClusterKey, cacheMissTracker, serverCfg.Webhook.CacheMissRetryAfter),
				),
			))

			if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
				klog.Exit(err, "unable to set up health check")
			}
			if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
				klog.Exit(err, "unable to set up ready check")
			}

			if err := mgr.Add(clusterCache); err != nil {
				klog.Exit(err, "unable to register cluster cache")
			}

			klog.Info("starting manager")
			if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
				klog.Exit(err, "problem running manager")
			}
		},
	}

	serverCfg = config.New()
	serverCfg.AddFlags(cmd.Flags())
	return cmd
}
