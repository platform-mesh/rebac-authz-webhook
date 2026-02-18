package storecache

import (
	"context"
	"net/url"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
)

type Provider interface {
	mcmanager.Runnable
	Get(string) (string, bool)
}

type storeCache struct {
	lock       sync.RWMutex
	cache      map[string]string
	orgsClient client.Client
}

func New(cfg *rest.Config) (*storeCache, error) {

	copiedCfg := rest.CopyConfig(cfg)
	parsed, err := url.Parse(copiedCfg.Host)
	if err != nil {
		return nil, err
	}

	parsed.Path = "/clusters/root:orgs"
	copiedCfg.Host = parsed.String()

	orgsClient, err := client.New(copiedCfg, client.Options{})
	if err != nil {
		return nil, err
	}

	return &storeCache{
		cache:      make(map[string]string),
		orgsClient: orgsClient,
	}, nil
}

func (s *storeCache) Get(clusterName string) (string, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	val, ok := s.cache[clusterName]
	return val, ok
}

// Engage implements manager.Runnable.
func (s *storeCache) Engage(ctx context.Context, name string, cl cluster.Cluster) error {
	klog.V(5).InfoS("Engaging cluster", "clusterName", name)

	// we need to retry here, since the cluster engagement happens very early and the
	// access to that workspace is not necessarily available immediately.
	var lc unstructured.Unstructured
	err := retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return ctx.Err() == nil
	}, func() error {
		// The schema does not necessarily contain information about the logical cluster, so we use an unstructured client to retrieve the annotations
		lc = unstructured.Unstructured{}
		lc.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "core.kcp.io",
			Version: "v1alpha1",
			Kind:    "LogicalCluster",
		})
		if err := cl.GetClient().Get(ctx, types.NamespacedName{Name: "cluster"}, &lc); err != nil {
			klog.V(5).ErrorS(err, "Failed to get LogicalCluster, will retry", "clusterName", name)
			return err
		}
		return nil
	})
	if err != nil {
		klog.ErrorS(err, "Failed to get LogicalCluster after retries", "clusterName", name)
		return err
	}

	annotationPath := lc.GetAnnotations()["kcp.io/path"]
	klog.V(5).InfoS("Retrieved logical cluster path", "clusterName", name, "path", annotationPath)

	const orgsPrefix = "root:orgs:"
	if !strings.HasPrefix(annotationPath, orgsPrefix) {
		klog.V(5).InfoS("Cluster path does not have orgs prefix, skipping", "clusterName", name, "path", annotationPath)
		return nil
	}
	orgName, _, _ := strings.Cut(annotationPath[len(orgsPrefix):], ":")
	klog.V(5).InfoS("Extracted org name", "clusterName", name, "orgName", orgName)

	// This is a deliberate choice to use unstructured to not have a dependency on the security operator api types
	// and also circumvent the missing schema issue
	var store unstructured.Unstructured
	store.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "core.platform-mesh.io",
		Version: "v1alpha1",
		Kind:    "Store",
	})
	err = s.orgsClient.Get(ctx, types.NamespacedName{Name: orgName}, &store)

	storeID, found, err := unstructured.NestedString(store.Object, "status", "storeId")
	if err != nil || !found {
		klog.V(5).ErrorS(err, "Failed to get storeId from Store status", "clusterName", name, "orgName", orgName, "found", found)
		return err
	}

	s.lock.Lock()
	s.cache[name] = storeID
	s.lock.Unlock()
	klog.V(5).InfoS("Cached storeId for cluster", "clusterName", name, "storeId", storeID)

	return nil
}

// Start implements manager.Runnable.
func (s *storeCache) Start(_ context.Context) error { // coverage-ignore
	return nil
}

var _ Provider = &storeCache{}
