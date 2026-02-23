package contextual_test

import (
	"context"
	"slices"
	"testing"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/clustercache"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/contextual"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"

	v1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestHandler(t *testing.T) {
	testCases := []struct {
		name              string
		req               authorization.Request
		res               authorization.Response
		mgrMocks func(mgr *mocks.Manager)
		fgaMocks          func(openfga *mocks.OpenFGAServiceClient)
		clusterCacheMocks func(cc *mocks.ClusterCacheProvider)
	}{
		{
			name: "should skip processing if clusterKey extra attrs not present",
			req:  authorization.Request{},
			res:  authorization.NoOpinion(),
		},
		{
			name: "should skip processing if cluster not found in cache",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						Extra: map[string]v1.ExtraValue{
							"authorization.kubernetes.io/cluster-name": {"a"},
						},
						ResourceAttributes: &v1.ResourceAttributes{},
					},
				},
			},
			res: authorization.NoOpinion(),
			clusterCacheMocks: func(cc *mocks.ClusterCacheProvider) {
				cc.EXPECT().Get("a").Return(clustercache.ClusterInfo{}, false)
			},
		},
		{
			name: "should skip processing if restmapper cannot resolve GVK",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						Extra: map[string]v1.ExtraValue{
							"authorization.kubernetes.io/cluster-name": {"a"},
						},
						ResourceAttributes: &v1.ResourceAttributes{
							Group:    "unknown.io",
							Version:  "v1",
							Resource: "unknowns",
						},
					},
				},
			},
			res: authorization.NoOpinion(),
			clusterCacheMocks: func(cc *mocks.ClusterCacheProvider) {
				rm := meta.NewDefaultRESTMapper([]schema.GroupVersion{})
				cc.EXPECT().Get("a").Return(clustercache.ClusterInfo{
					StoreID:         "store-id",
					RESTMapper:      rm,
					AccountName:     "origin-account",
					ParentClusterID: "origin",
				}, true)
			},
		},
		{
			name: "should process request non-parent, non-namespaced successfully",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						Extra: map[string]v1.ExtraValue{
							"authorization.kubernetes.io/cluster-name": {"a"},
						},
						ResourceAttributes: &v1.ResourceAttributes{
							Group:    "test.platform-mesh.io",
							Version:  "v1alpha1",
							Resource: "tests",
							Verb:     "get",
							Name:     "test-sample",
						},
					},
				},
			},
			res: authorization.Allowed(),
			clusterCacheMocks: func(cc *mocks.ClusterCacheProvider) {
				rm := meta.NewDefaultRESTMapper([]schema.GroupVersion{})

				gv := schema.GroupVersion{
					Group:   "test.platform-mesh.io",
					Version: "v1alpha1",
				}

				rm.AddSpecific(
					gv.WithKind("Test"),
					gv.WithResource("tests"),
					gv.WithResource("test"),
					meta.RESTScopeRoot,
				)

				cc.EXPECT().Get("a").Return(clustercache.ClusterInfo{
					StoreID:         "store-id",
					RESTMapper:      rm,
					AccountName:     "origin-account",
					ParentClusterID: "origin",
				}, true)
			},
			fgaMocks: func(openfga *mocks.OpenFGAServiceClient) {
				openfga.EXPECT().Check(mock.Anything, mock.Anything).RunAndReturn(
					func(ctx context.Context, in *openfgav1.CheckRequest, opts ...grpc.CallOption) (*openfgav1.CheckResponse, error) {

						tuples := in.ContextualTuples.TupleKeys

						contains := slices.ContainsFunc(tuples, func(tk *openfgav1.TupleKey) bool {
							return tk.Object == "test_platform-mesh_io_test:a/test-sample" &&
								tk.Relation == "parent" &&
								tk.User == "core_platform-mesh_io_account:origin/origin-account"
						})

						assert.True(t, contains)

						assert.Equal(t, "store-id", in.StoreId)
						assert.Equal(t, "test_platform-mesh_io_test:a/test-sample", in.TupleKey.Object)
						assert.Equal(t, "get", in.TupleKey.Relation)

						return &openfgav1.CheckResponse{
							Allowed: true,
						}, nil
					},
				)
			},
		},
		{
			name: "should process request non-parent, namespaced successfully",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						Extra: map[string]v1.ExtraValue{
							"authorization.kubernetes.io/cluster-name": {"a"},
						},
						ResourceAttributes: &v1.ResourceAttributes{
							Group:     "test.platform-mesh.io",
							Version:   "v1alpha1",
							Resource:  "tests",
							Verb:      "get",
							Name:      "test-sample",
							Namespace: "test-ns",
						},
					},
				},
			},
			res: authorization.Allowed(),
			clusterCacheMocks: func(cc *mocks.ClusterCacheProvider) {
				rm := meta.NewDefaultRESTMapper([]schema.GroupVersion{})

				gv := schema.GroupVersion{
					Group:   "test.platform-mesh.io",
					Version: "v1alpha1",
				}

				rm.AddSpecific(
					gv.WithKind("Test"),
					gv.WithResource("tests"),
					gv.WithResource("test"),
					meta.RESTScopeNamespace,
				)

				cc.EXPECT().Get("a").Return(clustercache.ClusterInfo{
					StoreID:         "store-id",
					RESTMapper:      rm,
					AccountName:     "origin-account",
					ParentClusterID: "origin",
				}, true)
			},
			fgaMocks: func(openfga *mocks.OpenFGAServiceClient) {
				openfga.EXPECT().Check(mock.Anything, mock.Anything).RunAndReturn(
					func(ctx context.Context, in *openfgav1.CheckRequest, opts ...grpc.CallOption) (*openfgav1.CheckResponse, error) {

						tuples := in.ContextualTuples.TupleKeys

						contains := slices.ContainsFunc(tuples, func(tk *openfgav1.TupleKey) bool {
							return tk.Object == "core_namespace:a/test-ns" &&
								tk.Relation == "parent" &&
								tk.User == "core_platform-mesh_io_account:origin/origin-account"
						})

						assert.True(t, contains)

						contains = slices.ContainsFunc(tuples, func(tk *openfgav1.TupleKey) bool {
							return tk.User == "core_namespace:a/test-ns" &&
								tk.Relation == "parent" &&
								tk.Object == "test_platform-mesh_io_test:a/test-sample"
						})

						assert.True(t, contains)

						assert.Equal(t, "store-id", in.StoreId)
						assert.Equal(t, "test_platform-mesh_io_test:a/test-sample", in.TupleKey.Object)
						assert.Equal(t, "get", in.TupleKey.Relation)

						return &openfgav1.CheckResponse{
							Allowed: true,
						}, nil
					},
				)
			},
		},
		{
			name: "should process request parent, namespaced successfully",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						Extra: map[string]v1.ExtraValue{
							"authorization.kubernetes.io/cluster-name": {"a"},
						},
						ResourceAttributes: &v1.ResourceAttributes{
							Group:     "test.platform-mesh.io",
							Version:   "v1alpha1",
							Resource:  "tests",
							Verb:      "list",
							Name:      "test-sample",
							Namespace: "test-ns",
						},
					},
				},
			},
			res: authorization.Allowed(),
			clusterCacheMocks: func(cc *mocks.ClusterCacheProvider) {
				rm := meta.NewDefaultRESTMapper([]schema.GroupVersion{})

				gv := schema.GroupVersion{
					Group:   "test.platform-mesh.io",
					Version: "v1alpha1",
				}

				rm.AddSpecific(
					gv.WithKind("Test"),
					gv.WithResource("tests"),
					gv.WithResource("test"),
					meta.RESTScopeNamespace,
				)

				cc.EXPECT().Get("a").Return(clustercache.ClusterInfo{
					StoreID:         "store-id",
					RESTMapper:      rm,
					AccountName:     "origin-account",
					ParentClusterID: "origin",
				}, true)
			},
			fgaMocks: func(openfga *mocks.OpenFGAServiceClient) {
				openfga.EXPECT().Check(mock.Anything, mock.Anything).RunAndReturn(
					func(ctx context.Context, in *openfgav1.CheckRequest, opts ...grpc.CallOption) (*openfgav1.CheckResponse, error) {

						tuples := in.ContextualTuples.TupleKeys

						contains := slices.ContainsFunc(tuples, func(tk *openfgav1.TupleKey) bool {
							return tk.Object == "core_namespace:a/test-ns" &&
								tk.Relation == "parent" &&
								tk.User == "core_platform-mesh_io_account:origin/origin-account"
						})

						assert.True(t, contains)

						assert.Equal(t, "store-id", in.StoreId)
						assert.Equal(t, "core_namespace:a/test-ns", in.TupleKey.Object)
						assert.Equal(t, "list_test_platform-mesh_io_tests", in.TupleKey.Relation)

						return &openfgav1.CheckResponse{
							Allowed: true,
						}, nil
					},
				)
			},
		},
		{
			name: "should process request parent, non-namespaced successfully",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						Extra: map[string]v1.ExtraValue{
							"authorization.kubernetes.io/cluster-name": {"a"},
						},
						ResourceAttributes: &v1.ResourceAttributes{
							Group:    "test.platform-mesh.io",
							Version:  "v1alpha1",
							Resource: "tests",
							Verb:     "list",
							Name:     "test-sample",
						},
					},
				},
			},
			res: authorization.Allowed(),
			clusterCacheMocks: func(cc *mocks.ClusterCacheProvider) {
				rm := meta.NewDefaultRESTMapper([]schema.GroupVersion{})

				gv := schema.GroupVersion{
					Group:   "test.platform-mesh.io",
					Version: "v1alpha1",
				}

				rm.AddSpecific(
					gv.WithKind("Test"),
					gv.WithResource("tests"),
					gv.WithResource("test"),
					meta.RESTScopeRoot,
				)

				cc.EXPECT().Get("a").Return(clustercache.ClusterInfo{
					StoreID:         "store-id",
					RESTMapper:      rm,
					AccountName:     "origin-account",
					ParentClusterID: "origin",
				}, true)
			},
			fgaMocks: func(openfga *mocks.OpenFGAServiceClient) {
				openfga.EXPECT().Check(mock.Anything, mock.Anything).RunAndReturn(
					func(ctx context.Context, in *openfgav1.CheckRequest, opts ...grpc.CallOption) (*openfgav1.CheckResponse, error) {

						tuples := in.ContextualTuples.TupleKeys

						contains := slices.ContainsFunc(tuples, func(tk *openfgav1.TupleKey) bool {
							return tk.Object == "test_platform-mesh_io_test:a/test-sample" &&
								tk.Relation == "parent" &&
								tk.User == "core_platform-mesh_io_account:origin/origin-account"
						})

						assert.True(t, contains)

						assert.Equal(t, "store-id", in.StoreId)
						assert.Equal(t, "core_platform-mesh_io_account:origin/origin-account", in.TupleKey.Object)
						assert.Equal(t, "list_test_platform-mesh_io_tests", in.TupleKey.Relation)

						return &openfgav1.CheckResponse{
							Allowed: true,
						}, nil
					},
				)
			},
		},
		{
			name: "should explicitly deny when OpenFGA returns allowed=false",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						Extra: map[string]v1.ExtraValue{
							"authorization.kubernetes.io/cluster-name": {"a"},
						},
						ResourceAttributes: &v1.ResourceAttributes{
							Group:    "test.platform-mesh.io",
							Version:  "v1alpha1",
							Resource: "tests",
							Verb:     "get",
							Name:     "test-sample",
						},
					},
				},
			},
			res: authorization.Denied(),
			k8sMocks: func(cl *mocks.Client, cluster *mocks.Cluster) {
				cl.EXPECT().
					Get(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(
						func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							acc := obj.(*v1alpha1.AccountInfo)
							*acc = v1alpha1.AccountInfo{
								Spec: v1alpha1.AccountInfoSpec{
									Account: v1alpha1.AccountLocation{OriginClusterId: "origin", Name: "origin-account"},
								},
							}
							return nil
						},
					)
			},
			rmpMocks: func(rmp *mocks.Provider) {
				rm := meta.NewDefaultRESTMapper([]schema.GroupVersion{})
				gv := schema.GroupVersion{Group: "test.platform-mesh.io", Version: "v1alpha1"}
				rm.AddSpecific(gv.WithKind("Test"), gv.WithResource("tests"), gv.WithResource("test"), meta.RESTScopeRoot)
				rmp.EXPECT().Get(mock.Anything).Return(rm, true)
			},
			fgaMocks: func(openfga *mocks.OpenFGAServiceClient) {
				openfga.EXPECT().Check(mock.Anything, mock.Anything).Return(&openfgav1.CheckResponse{Allowed: false}, nil)
			},
		},
		{
			name: "should allow cluster-scoped non-resource request when member",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						User: "alice",
						Extra: map[string]v1.ExtraValue{
							"authorization.kubernetes.io/cluster-name": {"a"},
						},
						NonResourceAttributes: &v1.NonResourceAttributes{Path: "/clusters/a/api"},
					},
				},
			},
			res: authorization.Allowed(),
			k8sMocks: func(cl *mocks.Client, cluster *mocks.Cluster) {
				cl.EXPECT().
					Get(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(
						func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							acc := obj.(*v1alpha1.AccountInfo)
							*acc = v1alpha1.AccountInfo{
								Spec: v1alpha1.AccountInfoSpec{
									FGA:     v1alpha1.FGAInfo{Store: v1alpha1.StoreInfo{Id: "store"}},
									Account: v1alpha1.AccountLocation{OriginClusterId: "origin", Name: "origin-account"},
								},
							}
							return nil
						},
					)
			},
			fgaMocks: func(openfga *mocks.OpenFGAServiceClient) {
				openfga.EXPECT().Check(mock.Anything, mock.Anything).RunAndReturn(
					func(ctx context.Context, in *openfgav1.CheckRequest, opts ...grpc.CallOption) (*openfgav1.CheckResponse, error) {
						assert.Equal(t, "store", in.StoreId)
						assert.Equal(t, "core_platform-mesh_io_account:origin/origin-account", in.TupleKey.Object)
						assert.Equal(t, "member", in.TupleKey.Relation)
						assert.Equal(t, "user:alice", in.TupleKey.User)
						return &openfgav1.CheckResponse{Allowed: true}, nil
					},
				)
			},
		},
		{
			name: "should deny cluster-scoped non-resource request when not a member",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						User: "alice",
						Extra: map[string]v1.ExtraValue{
							"authorization.kubernetes.io/cluster-name": {"a"},
						},
						NonResourceAttributes: &v1.NonResourceAttributes{Path: "/clusters/a/api"},
					},
				},
			},
			res: authorization.Denied(),
			k8sMocks: func(cl *mocks.Client, cluster *mocks.Cluster) {
				cl.EXPECT().
					Get(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(
						func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							acc := obj.(*v1alpha1.AccountInfo)
							*acc = v1alpha1.AccountInfo{
								Spec: v1alpha1.AccountInfoSpec{
									FGA:     v1alpha1.FGAInfo{Store: v1alpha1.StoreInfo{Id: "store"}},
									Account: v1alpha1.AccountLocation{OriginClusterId: "origin", Name: "origin-account"},
								},
							}
							return nil
						},
					)
			},
			fgaMocks: func(openfga *mocks.OpenFGAServiceClient) {
				openfga.EXPECT().Check(mock.Anything, mock.Anything).Return(&openfgav1.CheckResponse{Allowed: false}, nil)
			},
		},
		{
			name: "should deny cluster-scoped non-resource request when cluster is unknown",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						User: "alice",
						Extra: map[string]v1.ExtraValue{
							"authorization.kubernetes.io/cluster-name": {"a"},
						},
						NonResourceAttributes: &v1.NonResourceAttributes{Path: "/clusters/a/api"},
					},
				},
			},
			res: authorization.Denied(),
			mgrMocks: func(mgr *mocks.Manager) {
				mgr.EXPECT().GetCluster(mock.Anything, "a").Return(nil, assert.AnError)
			},
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {

			cc := mocks.NewClusterCacheProvider(t)
			if test.clusterCacheMocks != nil {
				test.clusterCacheMocks(cc)
			}

			rmp := mocks.NewProvider(t)
			if test.rmpMocks != nil {
				test.rmpMocks(rmp)
			}

			if test.mgrMocks != nil {
				test.mgrMocks(mgr)
			} else {
				mgr.EXPECT().GetCluster(mock.Anything, mock.Anything).Return(cluster, nil).Maybe()
			}
			cluster.EXPECT().GetClient().Return(client).Maybe()

			openfga := mocks.NewOpenFGAServiceClient(t)
			if test.fgaMocks != nil {
				test.fgaMocks(openfga)
			}

			h := contextual.New(openfga, cc, "authorization.kubernetes.io/cluster-name")

			ctx := t.Context()

			res := h.Handle(ctx, test.req)

			assert.Equal(t, test.res, res)
		})
	}
}
