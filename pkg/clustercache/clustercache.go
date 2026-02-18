package clustercache

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"

	"github.com/kcp-dev/logicalcluster/v3"
)

type ClusterInfo struct {
	StoreID         string
	RESTMapper      meta.RESTMapper
	AccountName     string
	ParentClusterID string
}

type Provider interface {
	mcmanager.Runnable
	Get(clusterName string) (ClusterInfo, bool)
}

type clusterCache struct {
	lock       sync.RWMutex
	cache      map[string]ClusterInfo
	orgsClient client.Client
}

func New(cfg *rest.Config) (*clusterCache, error) {
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

	return &clusterCache{
		cache:      make(map[string]ClusterInfo),
		orgsClient: orgsClient,
	}, nil
}

func NewWithClient(orgsClient client.Client) *clusterCache {
	return &clusterCache{
		cache:      make(map[string]ClusterInfo),
		orgsClient: orgsClient,
	}
}

func (c *clusterCache) Get(clusterName string) (ClusterInfo, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	val, ok := c.cache[clusterName]
	return val, ok
}

func (c *clusterCache) Engage(ctx context.Context, name string, cl cluster.Cluster) error {
	klog.V(5).InfoS("Engaging cluster", "clusterName", name)

	var lc unstructured.Unstructured
	err := retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return ctx.Err() == nil
	}, func() error {
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
	accountName := logicalcluster.NewPath(annotationPath).Base()

	parentClusterID, found, err := unstructured.NestedString(lc.Object, "spec", "owner", "cluster")
	if err != nil {
		klog.ErrorS(err, "Failed to get owner.cluster from LogicalCluster spec", "clusterName", name)
		return err
	}
	if !found {
		klog.Error("No owner.cluster found in LogicalCluster spec", "clusterName", name)
		return errors.New("owner.cluster not found in LogicalCluster spec")
	}

	var store unstructured.Unstructured
	store.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "core.platform-mesh.io",
		Version: "v1alpha1",
		Kind:    "Store",
	})
	err = c.orgsClient.Get(ctx, types.NamespacedName{Name: orgName}, &store)
	if err != nil {
		klog.ErrorS(err, "Failed to get Store for org", "clusterName", name, "orgName", orgName)
		return err
	}

	storeID, found, err := unstructured.NestedString(store.Object, "status", "storeId")
	if err != nil {
		klog.ErrorS(err, "Failed to get storeId from Store status", "clusterName", name, "orgName", orgName)
		return err
	}
	if !found {
		klog.V(5).InfoS("storeId not found in Store status", "clusterName", name, "orgName", orgName)
		return errors.New("storeId not found in Store status")
	}

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

	c.lock.Lock()
	c.cache[name] = ClusterInfo{
		StoreID:         storeID,
		RESTMapper:      restMapper,
		AccountName:     accountName,
		ParentClusterID: parentClusterID,
	}
	c.lock.Unlock()

	klog.V(5).InfoS("Cached cluster info",
		"clusterName", name,
		"storeId", storeID,
		"accountName", accountName,
		"parentClusterID", parentClusterID)

	return nil
}

func (c *clusterCache) Start(_ context.Context) error { // coverage-ignore
	return nil
}

var _ Provider = &clusterCache{}
