package storecache

import (
	"context"
	"testing"

	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestStoreCache_Engage(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantCached bool
		wantOrg    string
	}{
		{
			name:       "full path with nested workspaces",
			path:       "root:orgs:myorg:ws:child",
			wantCached: true,
			wantOrg:    "myorg",
		},
		{
			name:       "path with only org",
			path:       "root:orgs:myorg",
			wantCached: true,
			wantOrg:    "myorg",
		},
		{
			name:       "root only - skipped",
			path:       "root",
			wantCached: false,
		},
		{
			name:       "root:platform-mesh-system - skipped",
			path:       "root:platform-mesh-system",
			wantCached: false,
		},
		{
			name:       "root:provider:something - skipped",
			path:       "root:provider:something",
			wantCached: false,
		},
		{
			name:       "empty path - skipped",
			path:       "",
			wantCached: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := mocks.NewCluster(t)
			k8sClient := mocks.NewClient(t)

			cl.EXPECT().GetClient().Return(k8sClient)
			k8sClient.EXPECT().Get(mock.Anything, types.NamespacedName{Name: "cluster"}, mock.Anything).
				Run(func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) {
					lc := obj.(*unstructured.Unstructured)
					lc.SetAnnotations(map[string]string{
						"kcp.io/path": tt.path,
					})
				}).
				Return(nil)

			orgsClient := mocks.NewClient(t)

			if tt.wantCached {
				orgsClient.EXPECT().Get(mock.Anything, types.NamespacedName{Name: tt.wantOrg}, mock.Anything).
					Run(func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) {
						u := obj.(*unstructured.Unstructured)
						u.Object = map[string]any{
							"status": map[string]any{
								"storeId": tt.wantOrg + "-store-id",
							},
						}
					}).
					Return(nil)
			}

			sc := &storeCache{
				cache:      make(map[string]string),
				orgsClient: orgsClient,
			}
			err := sc.Engage(t.Context(), "test-cluster", cl)
			assert.NoError(t, err)

			storeID, found := sc.Get("test-cluster")
			assert.Equal(t, tt.wantCached, found)
			if tt.wantCached {
				assert.Equal(t, tt.wantOrg+"-store-id", storeID)
			}
		})
	}
}
