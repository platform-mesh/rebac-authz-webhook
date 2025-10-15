package restmapper

import (
	"context"
	"net/url"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
)

type Provider interface {
	mcmanager.Runnable
	Get(string) (meta.RESTMapper, bool)
}

type restMapper struct {
	lock    sync.RWMutex
	mappers map[string]meta.RESTMapper
}

func New() *restMapper {
	return &restMapper{}
}

func (r *restMapper) Get(clusterName string) (meta.RESTMapper, bool) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	m, ok := r.mappers[clusterName]
	return m, ok
}

// Engage implements manager.Runnable.
func (r *restMapper) Engage(ctx context.Context, name string, cl cluster.Cluster) error {

	cfg := rest.CopyConfig(cl.GetConfig())

	parsed, err := url.Parse(cfg.Host)
	if err != nil {
		return err
	}

	path, err := url.JoinPath("clusters", name)
	if err != nil {
		return err
	}

	parsed.Path = path
	cfg.Host = parsed.String()

	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return err
	}

	restMapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	if err != nil {
		return err
	}

	r.lock.Lock()
	r.mappers[name] = restMapper
	r.lock.Unlock()

	return nil
}

// Start implements manager.Runnable.
func (r *restMapper) Start(_ context.Context) error {
	r.mappers = make(map[string]meta.RESTMapper)
	return nil
}

var _ Provider = &restMapper{}
