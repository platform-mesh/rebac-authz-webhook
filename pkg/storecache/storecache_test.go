package storecache

import (
	"context"
	"errors"
	"testing"

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
			name:    "valid URL",
			host:    "https://example.com",
			wantErr: false,
		},
		{
			name:    "invalid URL",
			host:    "://invalid-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &rest.Config{Host: tt.host}
			sc, err := New(cfg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, sc)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, sc)
				assert.NotNil(t, sc.cache)
				assert.NotNil(t, sc.orgsClient)
			}
		})
	}
}

func TestStoreCache_Engage(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		wantCached     bool
		wantOrg        string
		lcGetErr       error
		cancelCtx      bool
		storeIdMissing bool
		wantErr        bool
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
		{
			name:      "logical cluster get error with cancelled context",
			path:      "root:orgs:myorg",
			lcGetErr:  errors.New("connection refused"),
			cancelCtx: true,
			wantErr:   true,
		},
		{
			name:           "storeId not found in status",
			path:           "root:orgs:myorg",
			wantOrg:        "myorg",
			storeIdMissing: true,
			wantCached:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := mocks.NewCluster(t)
			k8sClient := mocks.NewClient(t)

			cl.EXPECT().GetClient().Return(k8sClient)

			ctx := t.Context()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			k8sClient.EXPECT().Get(mock.Anything, types.NamespacedName{Name: "cluster"}, mock.Anything).
				Run(func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) {
					lc := obj.(*unstructured.Unstructured)
					lc.SetAnnotations(map[string]string{
						"kcp.io/path": tt.path,
					})
				}).
				Return(tt.lcGetErr)

			orgsClient := mocks.NewClient(t)

			if tt.wantCached || tt.storeIdMissing {
				orgsClient.EXPECT().Get(mock.Anything, types.NamespacedName{Name: tt.wantOrg}, mock.Anything).
					Run(func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) {
						u := obj.(*unstructured.Unstructured)
						if tt.storeIdMissing {
							u.Object = map[string]any{
								"status": map[string]any{},
							}
						} else {
							u.Object = map[string]any{
								"status": map[string]any{
									"storeId": tt.wantOrg + "-store-id",
								},
							}
						}
					}).
					Return(nil)
			}

			sc := &storeCache{
				cache:      make(map[string]string),
				orgsClient: orgsClient,
			}
			err := sc.Engage(ctx, "test-cluster", cl)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			storeID, found := sc.Get("test-cluster")
			assert.Equal(t, tt.wantCached, found)
			if tt.wantCached {
				assert.Equal(t, tt.wantOrg+"-store-id", storeID)
			}
		})
	}
}
