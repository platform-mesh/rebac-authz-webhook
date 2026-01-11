package restmapper

import (
	"context"
	"net/url"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type Provider interface {
	mcmanager.Runnable
	Get(string) (meta.RESTMapper, bool)
}

type restMapper struct {
	lock    sync.RWMutex
	mappers map[string]meta.RESTMapper
}

func New() *restMapper { // coverage-ignore
	return &restMapper{
		mappers: make(map[string]meta.RESTMapper),
	}
}

func (r *restMapper) Get(clusterName string) (meta.RESTMapper, bool) { // coverage-ignore
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

	klog.V(5).InfoS("Creating RESTMapper for cluster", "cluster", name, "host", cfg.Host)

	restMapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	if err != nil {
		return err
	}

	r.lock.Lock()
	r.mappers[name] = restMapper
	r.lock.Unlock()

	// TODO: react on context cancellation and remove the restMapper from the map

	return nil
}

// Start implements manager.Runnable.
func (r *restMapper) Start(_ context.Context) error { // coverage-ignore
	return nil
}

var _ Provider = &restMapper{}
