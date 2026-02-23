package cmd

import (
	"crypto/tls"
	"net/http"
	"net/url"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization/union"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/clustercache"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/config"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/contextual"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/nonresourceattributes"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/orgs"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/kcp-dev/logicalcluster/v3"
	"github.com/kcp-dev/multicluster-provider/apiexport"
	kcpclientset "github.com/kcp-dev/sdk/client/clientset/versioned/cluster"
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

			endpointSliceName := serverCfg.APIExportEndpointSliceName
			klog.InfoS("using endpoint slice name", "name", endpointSliceName)

			provider, err := apiexport.New(restCfg, endpointSliceName, apiexport.Options{
				Scheme: scheme,
			})
			if err != nil {
				klog.Exit(err, "unable to construct cluster provider")
			}

			// Use Root KCP config for manager
			mgr, err := mcmanager.New(restCfg, provider, mcmanager.Options{
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

			rootCfg := rest.CopyConfig(restCfg)
			rootURL, err := url.Parse(rootCfg.Host)
			if err != nil {
				klog.Exit(err, "failed to parse root cluster URL")
			}

			rootURL.Path = ""
			rootCfg.Host = rootURL.String()

			clusterClient, err := kcpclientset.NewForConfig(rootCfg)
			if err != nil {
				klog.Exit(err, "failed to construct cluster client")
			}

			orgsCluster, err := clusterClient.Cluster(logicalcluster.NewPath("root:orgs")).CoreV1alpha1().LogicalClusters().Get(ctx, "cluster", metav1.GetOptions{})
			if err != nil {
				klog.Exit(err, "failed to get orgs cluster")
			}

			orgsClusterID := logicalcluster.From(orgsCluster)
			klog.InfoS("found orgs cluster", "name", orgsCluster.Name, "cluster", orgsClusterID.String())

			clusterCache, err := clustercache.New(restCfg)
			if err != nil {
				klog.Exit(err, "failed to create cluster cache")
			}

			extraAttrClusterKey := serverCfg.Webhook.ClusterKey

			mgr.GetWebhookServer().Register("/authz", authorization.New(
				klog.NewKlogr(),
				union.New(
					nonresourceattributes.New(serverCfg.Webhook.AllowedNonResourcePrefixes...),
					orgs.New(fga, extraAttrClusterKey, orgsClusterID.String(), storeRes.Stores[0].Id),
					contextual.New(fga, clusterCache, extraAttrClusterKey),
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
