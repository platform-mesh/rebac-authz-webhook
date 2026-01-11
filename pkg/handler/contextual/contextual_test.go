package contextual_test

import (
	"context"
	"slices"
	"testing"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/platform-mesh/account-operator/api/v1alpha1"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/contextual"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestHandler(t *testing.T) {
	testCases := []struct {
		name     string
		req      authorization.Request
		res      authorization.Response
		fgaMocks func(openfga *mocks.OpenFGAServiceClient)
		k8sMocks func(client *mocks.Client, cluster *mocks.Cluster)
		rmpMocks func(rmp *mocks.Provider)
	}{
		{
			name: "should skip processing if no extra attrs present",
			req:  authorization.Request{},
			res:  authorization.NoOpinion(),
		},
		{
			name: "should skip processing if clusterKey extra attrs not present",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						Extra: map[string]v1.ExtraValue{
							"a": {"b"},
						},
					},
				},
			},
			res: authorization.NoOpinion(),
		},
		{
			name: "should skip processing if accountinfo cannot be retrieved",
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
			k8sMocks: func(client *mocks.Client, cluster *mocks.Cluster) {
				client.EXPECT().
					Get(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(assert.AnError)

			},
		},
		{
			name: "should skip processing if restmapper cannot be retrieved",
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
			k8sMocks: func(cl *mocks.Client, cluster *mocks.Cluster) {
				cl.EXPECT().
					Get(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(
						func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							acc := obj.(*v1alpha1.AccountInfo)

							*acc = v1alpha1.AccountInfo{
								Spec: v1alpha1.AccountInfoSpec{
									Account: v1alpha1.AccountLocation{
										OriginClusterId: "origin",
										Name:            "origin-account",
									},
								},
							}
							return nil
						},
					)

			},
			rmpMocks: func(rmp *mocks.Provider) {
				rmp.EXPECT().Get(mock.Anything).Return(nil, false)
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
			k8sMocks: func(cl *mocks.Client, cluster *mocks.Cluster) {
				cl.EXPECT().
					Get(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(
						func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							acc := obj.(*v1alpha1.AccountInfo)

							*acc = v1alpha1.AccountInfo{
								Spec: v1alpha1.AccountInfoSpec{
									Account: v1alpha1.AccountLocation{
										OriginClusterId: "origin",
										Name:            "origin-account",
									},
								},
							}
							return nil
						},
					)

			},
			rmpMocks: func(rmp *mocks.Provider) {
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

				rmp.EXPECT().Get(mock.Anything).Return(rm, true)
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
			k8sMocks: func(cl *mocks.Client, cluster *mocks.Cluster) {
				cl.EXPECT().
					Get(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(
						func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							acc := obj.(*v1alpha1.AccountInfo)

							*acc = v1alpha1.AccountInfo{
								Spec: v1alpha1.AccountInfoSpec{
									Account: v1alpha1.AccountLocation{
										OriginClusterId: "origin",
										Name:            "origin-account",
									},
								},
							}
							return nil
						},
					)
			},
			rmpMocks: func(rmp *mocks.Provider) {
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

				rmp.EXPECT().Get(mock.Anything).Return(rm, true)
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
			k8sMocks: func(cl *mocks.Client, cluster *mocks.Cluster) {
				cl.EXPECT().
					Get(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(
						func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							acc := obj.(*v1alpha1.AccountInfo)

							*acc = v1alpha1.AccountInfo{
								Spec: v1alpha1.AccountInfoSpec{
									Account: v1alpha1.AccountLocation{
										OriginClusterId: "origin",
										Name:            "origin-account",
									},
								},
							}
							return nil
						},
					)
			},
			rmpMocks: func(rmp *mocks.Provider) {
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

				rmp.EXPECT().Get(mock.Anything).Return(rm, true)
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
			k8sMocks: func(cl *mocks.Client, cluster *mocks.Cluster) {
				cl.EXPECT().
					Get(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(
						func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							acc := obj.(*v1alpha1.AccountInfo)

							*acc = v1alpha1.AccountInfo{
								Spec: v1alpha1.AccountInfoSpec{
									Account: v1alpha1.AccountLocation{
										OriginClusterId: "origin",
										Name:            "origin-account",
									},
								},
							}
							return nil
						},
					)
			},
			rmpMocks: func(rmp *mocks.Provider) {
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

				rmp.EXPECT().Get(mock.Anything).Return(rm, true)
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

						assert.Equal(t, "core_platform-mesh_io_account:origin/origin-account", in.TupleKey.Object)
						assert.Equal(t, "list_test_platform-mesh_io_tests", in.TupleKey.Relation)

						return &openfgav1.CheckResponse{
							Allowed: true,
						}, nil
					},
				)
			},
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {

			mgr := mocks.NewManager(t)
			cluster := mocks.NewCluster(t)
			client := mocks.NewClient(t)
			if test.k8sMocks != nil {
				test.k8sMocks(client, cluster)
			}

			rmp := mocks.NewProvider(t)
			if test.rmpMocks != nil {
				test.rmpMocks(rmp)
			}

			mgr.EXPECT().GetCluster(mock.Anything, mock.Anything).Return(cluster, nil).Maybe()
			cluster.EXPECT().GetClient().Return(client).Maybe()

			openfga := mocks.NewOpenFGAServiceClient(t)
			if test.fgaMocks != nil {
				test.fgaMocks(openfga)
			}

			h := contextual.New(mgr, openfga, rmp, "authorization.kubernetes.io/cluster-name")

			ctx := t.Context()

			res := h.Handle(ctx, test.req)

			assert.Equal(t, test.res, res)
		})
	}
}
