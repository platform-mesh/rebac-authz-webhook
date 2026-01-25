package cmd

import (
	"crypto/tls"
	"net/http"
	"net/url"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization/union"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/contextual"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/nonresourceattributes"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/orgs"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/restmapper"
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

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts the authorization webhook server",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()

		ctrl.SetLogger(klog.NewKlogr())

		// Use ctrl.GetConfigOrDie() which respects KUBECONFIG environment variable
		// This allows the operator to provide a kubeconfig secret that points to Root KCP API Server
		// The KUBECONFIG env var is set in the Helm chart deployment template
		// This is needed because cache.New() requires API Server Discovery which only works with Root KCP API Server
		// The Virtual Workspace kubeconfig doesn't support API Server Discovery for root KCP CRDs like APIExportEndpointSlice
		restCfg := ctrl.GetConfigOrDie()

		restCfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
			return otelhttp.NewTransport(rt)
		})

		// Use Root KCP config for apiexport.New() (like iam-service)
		// cache.New() inside the provider needs Root KCP API Server to discover APIExportEndpointSlice
		// The provider will then use Virtual Workspace URLs from the slice for actual cluster access
		endpointSliceName := serverCfg.KCP.APIExportEndpointSliceName
		if endpointSliceName == "" {
			// Default to "core.platform-mesh.io" if not specified (like iam-service)
			// Auto-discovery with empty string doesn't work correctly with the cache filter
			endpointSliceName = "core.platform-mesh.io"
			klog.Info("no endpoint slice name provided, using default", "name", endpointSliceName)
		} else {
			klog.InfoS("using endpoint slice name", "name", endpointSliceName)
		}

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
				BindAddress: defaultCfg.Metrics.BindAddress,
				TLSOpts: []func(*tls.Config){
					func(c *tls.Config) {
						klog.Info("disabling http/2")
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

		mapperProvider := restmapper.New()

		extraAttrClusterKey := serverCfg.Webhook.ClusterKey

		mgr.GetWebhookServer().Register("/authz", authorization.New(
			klog.NewKlogr(),
			union.New(
				nonresourceattributes.New(serverCfg.Webhook.AllowedNonResourcePrefixes...),
				orgs.New(fga, extraAttrClusterKey, orgsClusterID.String(), storeRes.Stores[0].Id),
				contextual.New(mgr, fga, mapperProvider, extraAttrClusterKey),
			),
		))

		if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
			klog.Exit(err, "unable to set up health check")
		}
		if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
			klog.Exit(err, "unable to set up ready check")
		}

		if err := mgr.Add(mapperProvider); err != nil {
			klog.Exit(err, "unable to register rest mapper provider")
		}

		klog.Info("starting manager")
		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			klog.Exit(err, "problem running manager")
		}
	},
}
