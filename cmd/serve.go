package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	kcpcorev1alpha1 "github.com/kcp-dev/kcp/sdk/apis/core/v1alpha1"
	"github.com/kcp-dev/multicluster-provider/apiexport"
	accountsv1alpha1 "github.com/platform-mesh/account-operator/api/v1alpha1"
	commonconfig "github.com/platform-mesh/golang-commons/config"
	pmeshcontext "github.com/platform-mesh/golang-commons/context"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	"github.com/platform-mesh/golang-commons/logger"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/clientcmd"

	tenancyv1alpha1 "github.com/kcp-dev/kcp/sdk/apis/tenancy/v1alpha1"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/client"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/config"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler"

	kcpclient "github.com/kcp-dev/kcp/pkg/client/clientset/versioned"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	defaultCfg *commonconfig.CommonServiceConfig
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts the authorization webhook server",
	Run: func(cmd *cobra.Command, args []string) {
		serve()
	},
}

func serve() { // coverage-ignore

	ctrl.SetLogger(log.ComponentLogger("controller-runtime").Logr())

	ctx, _, shutdown := pmeshcontext.StartContext(log, serverCfg, defaultCfg.ShutdownTimeout)
	defer shutdown()

	log.Info().Msg("Starting the authorization webhook server")

	restCfg := ctrl.GetConfigOrDie()
	restCfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt)
	})

	mgr, provider := initializeMultiClusterManager(ctx, restCfg, log, serverCfg, defaultCfg)
	fga := client.MustCreateInClusterClient(serverCfg.OpenFGA.Addr)

	kcpScheme := runtime.NewScheme()
	utilruntime.Must(kcpcorev1alpha1.AddToScheme(kcpScheme))
	utilruntime.Must(accountsv1alpha1.AddToScheme(kcpScheme))
	utilruntime.Must(tenancyv1alpha1.AddToScheme(kcpScheme))

	srv := mgr.GetWebhookServer()
	cmw := &ContextMiddleware{Logger: log}

	orgsWorkspaceID, err := getOrgWorkspaceID(ctx, restCfg)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot get organization's workspace ID")
	}

	orgsStoreID, err := getOrgStoreID(fga, "orgs")
	if err != nil {
		log.Fatal().Err(err).Msg("cannot get organization's store ID")
	}

	authHandler, err := handler.NewAuthorizationHandler(fga, mgr, serverCfg.Kcp.AccountInfoName, orgsStoreID, orgsWorkspaceID)
	if err != nil {
		log.Fatal().Err(err).Msg("could not create authorization handler")
	}

	srv.Register("/authz", cmw.Middleware(authHandler))

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Fatal().Err(err).Msg("unable to set up health check")
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Fatal().Err(err).Msg("unable to set up ready check")
	}

	log.Info().Msg("Starting provider")
	go func() {
		if err := provider.Run(ctx, mgr); err != nil {
			log.Fatal().Err(err).Msg("unable to run provider")
		}
	}()
	log.Info().Msg("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Fatal().Err(err).Msg("problem running manager")
	}

}

func initializeMultiClusterManager(ctx context.Context, restCfg *rest.Config, log *logger.Logger, serviceCfg config.Config, defaultConfig *commonconfig.CommonServiceConfig) (mcmanager.Manager, *apiexport.Provider) {
	log.Info().Msg("Initializing multicluster manager")
	kubeconfigPath := serviceCfg.Kcp.KubeconfigPath
	kcpCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		log.Fatal().Err(err).Str("controller", "ContentConfiguration").Msg("unable to construct cluster provider")
	}
	kcpCfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt)
	})

	if serverCfg.Kcp.ClusterURL != "" {
		kcpCfg.Host = serverCfg.Kcp.ClusterURL
	}

	provider, err := apiexport.New(kcpCfg, apiexport.Options{
		Scheme: scheme,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("unable to construct cluster provider")
	}

	mgr, err := mcmanager.New(kcpCfg, provider, mcmanager.Options{
		Scheme:        scheme,
		Logger:        log.Logr(),
		WebhookServer: webhook.NewServer(webhook.Options{CertDir: serverCfg.Webhook.CertDir}),
		Metrics: metricsserver.Options{
			BindAddress: defaultConfig.Metrics.BindAddress,
			TLSOpts: []func(*tls.Config){
				func(c *tls.Config) {
					log.Info().Msg("disabling http/2")
					c.NextProtos = []string{"http/1.1"}
				},
			},
		},
		BaseContext:                   func() context.Context { return ctx },
		HealthProbeBindAddress:        defaultConfig.HealthProbeBindAddress,
		LeaderElection:                defaultConfig.LeaderElection.Enabled,
		LeaderElectionID:              "rebac-authz-webhook.platform-mesh.io",
		LeaderElectionReleaseOnCancel: true,
		LeaderElectionConfig:          restCfg,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("unable to set up overall controller manager")
	}

	return mgr, provider
}

type ContextMiddleware struct {
	Logger *logger.Logger
}

func (c *ContextMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := logger.SetLoggerInContext(r.Context(), c.Logger)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getOrgWorkspaceID(ctx context.Context, restCfg *rest.Config) (string, error) {
	kcpClientSet, err := kcpclient.NewForConfig(restCfg)
	if err != nil {
		return "", err
	}
	orgWorkspace, err := kcpClientSet.TenancyV1alpha1().Workspaces().Get(ctx, "orgs", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return orgWorkspace.Spec.Cluster, nil
}

func getOrgStoreID(fga openfgav1.OpenFGAServiceClient, storeName string) (string, error) {
	stores, err := fga.ListStores(context.TODO(), &openfgav1.ListStoresRequest{})
	if err != nil {
		return "", err
	}

	for _, store := range stores.Stores {
		if store.Name == storeName {
			return store.Id, nil
		}
	}
	return "", fmt.Errorf("store %s doesn't exist", storeName)
}
