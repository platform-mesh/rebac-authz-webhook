package mapperprovider

import (
	"context"
	"net/url"
	"path"
	"sync"

	apisv1alpha1 "github.com/kcp-dev/kcp/sdk/apis/apis/v1alpha1"
	"github.com/kcp-dev/logicalcluster/v3"
	"github.com/platform-mesh/golang-commons/logger"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type MapperProviders struct {
	lock     sync.RWMutex
	clusters map[logicalcluster.Name]meta.RESTMapper
}

func New() *MapperProviders {
	return &MapperProviders{
		clusters: make(map[logicalcluster.Name]meta.RESTMapper),
	}
}

func (mps *MapperProviders) GetMapper(clusterName logicalcluster.Name) (meta.RESTMapper, bool) {
	mps.lock.RLock()
	defer mps.lock.RUnlock()

	mapper, ok := mps.clusters[clusterName]
	return mapper, ok
}

func Run(ctx context.Context, kcpCfg *rest.Config, mps *MapperProviders, c cache.Cache, log *logger.Logger) error {
	inf, err := c.GetInformer(ctx, &apisv1alpha1.APIBinding{}, cache.BlockUntilSynced(false))
	if err != nil {
		return err
	}

	_, err = inf.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			cobj, ok := obj.(client.Object)
			if !ok {
				klog.Errorf("unexpected object type %T", obj)
				return
			}

			clusterName := logicalcluster.From(cobj)

			// fast path: cluster exists already, there is nothing to do.
			mps.lock.RLock()
			if _, ok := mps.clusters[clusterName]; ok {
				mps.lock.RUnlock()
				return
			}
			mps.lock.RUnlock()

			// slow path: take write lock to add a new cluster (unless it appeared in the meantime).
			mps.lock.Lock()
			if _, ok := mps.clusters[clusterName]; ok {
				mps.lock.Unlock()
				return
			}

			cfg := rest.CopyConfig(kcpCfg)
			parsed, err := url.Parse(cfg.Host)
			if err != nil {
				mps.lock.Unlock()
				log.Fatal().Err(err).Msg("unable to parse host URL for cluster")
			}

			parsed.Path = path.Join("clusters", string(clusterName))

			cfg.Host = parsed.String()

			httpClient, err := rest.HTTPClientFor(cfg)
			if err != nil {
				mps.lock.Unlock()
				log.Fatal().Err(err).Msg("unable to create HTTP client for cluster")
			}

			mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
			if err != nil {
				mps.lock.Unlock()
				log.Fatal().Err(err).Msg("unable to create REST mapper for cluster")
			}

			mps.clusters[clusterName] = mapper
			mps.lock.Unlock()
		},
		DeleteFunc: func(obj any) {
			cobj, ok := obj.(client.Object)
			if !ok {
				tombstone, ok := obj.(toolscache.DeletedFinalStateUnknown)
				if !ok {
					klog.Errorf("Couldn't get object from tombstone %#v", obj)
					return
				}
				cobj, ok = tombstone.Obj.(client.Object)
				if !ok {
					klog.Errorf("Tombstone contained object that is not expected %#v", obj)
					return
				}
			}

			clusterName := logicalcluster.From(cobj)

			mps.lock.Lock()
			delete(mps.clusters, clusterName)
			mps.lock.Unlock()
		},
	})

	return err
}
