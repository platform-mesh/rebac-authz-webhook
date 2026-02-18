package clustercache_test

import (
	"context"
	"errors"
	"testing"

	"github.com/platform-mesh/rebac-authz-webhook/pkg/clustercache"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{
			name: "valid URL",
			host: "https://example.com",
		},
		{
			name:    "invalid URL",
			host:    "://invalid-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc, err := clustercache.New(&rest.Config{Host: tt.host})
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, cc)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cc)
			}
		})
	}
}

func TestClusterCache_Engage(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		ownerCluster    string
		setupOrgsClient func(*mocks.Client)
		setupCluster    func(*mocks.Cluster)
		lcGetErr        error
		wantCached      bool
		wantAccountName string
		wantErr         bool
	}{
		{
			name:         "caches cluster info successfully",
			path:         "root:orgs:myorg:ws:child",
			ownerCluster: "parent-cluster-id",
			setupOrgsClient: func(c *mocks.Client) {
				setupStoreGet(c, "myorg", "myorg-store-id")
			},
			setupCluster:    func(c *mocks.Cluster) { c.EXPECT().GetConfig().Return(&rest.Config{Host: "https://example.com"}) },
			wantCached:      true,
			wantAccountName: "child",
		},
		{
			name:    "returns error when owner missing",
			path:    "root:orgs:myorg",
			wantErr: true,
		},
		{
			name:       "skips non-org path",
			path:       "root:platform-mesh-system",
			wantCached: false,
		},
		{
			name:     "returns error on logical cluster get failure",
			path:     "root:orgs:myorg",
			lcGetErr: errors.New("connection refused"),
			wantErr:  true,
		},
		{
			name:         "returns error when storeId missing",
			path:         "root:orgs:myorg",
			ownerCluster: "parent-cluster",
			setupOrgsClient: func(c *mocks.Client) {
				setupStoreGetMissing(c, "myorg")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := mocks.NewCluster(t)
			k8sClient := mocks.NewClient(t)
			orgsClient := mocks.NewClient(t)

			cl.EXPECT().GetClient().Return(k8sClient)

			ctx := t.Context()
			if tt.lcGetErr != nil {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			k8sClient.EXPECT().Get(mock.Anything, types.NamespacedName{Name: "cluster"}, mock.Anything, mock.Anything).
				Run(func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) {
					lc := obj.(*unstructured.Unstructured)
					lc.SetAnnotations(map[string]string{"kcp.io/path": tt.path})
					if tt.ownerCluster != "" {
						lc.Object["spec"] = map[string]interface{}{
							"owner": map[string]interface{}{"cluster": tt.ownerCluster},
						}
					}
				}).
				Return(tt.lcGetErr)

			if tt.setupOrgsClient != nil {
				tt.setupOrgsClient(orgsClient)
			}
			if tt.setupCluster != nil {
				tt.setupCluster(cl)
			}

			cc := clustercache.NewWithClient(orgsClient)
			err := cc.Engage(ctx, "test-cluster", cl)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			info, found := cc.Get("test-cluster")
			assert.Equal(t, tt.wantCached, found)
			if tt.wantCached {
				assert.Equal(t, tt.wantAccountName, info.AccountName)
				assert.Equal(t, tt.ownerCluster, info.ParentClusterID)
				assert.NotNil(t, info.RESTMapper)
			}
		})
	}
}

func TestClusterCache_Get_NotFound(t *testing.T) {
	cc := clustercache.NewWithClient(nil)
	info, found := cc.Get("non-existing")
	assert.False(t, found)
	assert.Empty(t, info.StoreID)
}

func setupStoreGet(c *mocks.Client, orgName, storeID string) {
	c.EXPECT().Get(mock.Anything, types.NamespacedName{Name: orgName}, mock.Anything, mock.Anything).
		Run(func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) {
			obj.(*unstructured.Unstructured).Object = map[string]any{
				"status": map[string]any{"storeId": storeID},
			}
		}).
		Return(nil)
}

func setupStoreGetMissing(c *mocks.Client, orgName string) {
	c.EXPECT().Get(mock.Anything, types.NamespacedName{Name: orgName}, mock.Anything, mock.Anything).
		Run(func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) {
			obj.(*unstructured.Unstructured).Object = map[string]any{
				"status": map[string]any{},
			}
		}).
		Return(nil)
}
