package orgs_test

import (
	"context"
	"testing"

	kcpcorev1alpha "github.com/kcp-dev/sdk/apis/core/v1alpha1"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/mocks"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/orgs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	v1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestHandler(t *testing.T) {
	testCases := []struct {
		name     string
		req      authorization.Request
		res      authorization.Response
		fgaMocks func(openfga *mocks.OpenFGAServiceClient)
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
			name: "should skip processing if clusterKey does not match orgID",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						Extra: map[string]v1.ExtraValue{
							"authorization.kubernetes.io/cluster-name": {"b"},
						},
					},
				},
			},
			res: authorization.NoOpinion(),
		},
		{
			name: "should allow if fga check allows",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						Extra: map[string]v1.ExtraValue{
							"authorization.kubernetes.io/cluster-name": {"a"},
						},
						ResourceAttributes: &v1.ResourceAttributes{
							Group:    "a",
							Version:  "b",
							Resource: "c",
						},
					},
				},
			},
			res: authorization.Allowed(),
			fgaMocks: func(openfga *mocks.OpenFGAServiceClient) {
				openfga.EXPECT().Check(mock.Anything, mock.Anything).
					Return(&openfgav1.CheckResponse{
						Allowed: true,
					}, nil)
			},
		},
		{
			name: "should abort if fga check denies",
			req: authorization.Request{
				SubjectAccessReview: v1.SubjectAccessReview{
					Spec: v1.SubjectAccessReviewSpec{
						Extra: map[string]v1.ExtraValue{
							"authorization.kubernetes.io/cluster-name": {"a"},
						},
						ResourceAttributes: &v1.ResourceAttributes{
							Group:    "a",
							Version:  "b",
							Resource: "c",
						},
					},
				},
			},
			res: authorization.Aborted(),
			fgaMocks: func(openfga *mocks.OpenFGAServiceClient) {
				openfga.EXPECT().Check(mock.Anything, mock.Anything).
					Return(&openfgav1.CheckResponse{
						Allowed: false,
					}, nil)
			},
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {

			openfga := mocks.NewOpenFGAServiceClient(t)
			mgr := mocks.NewManager(t)
			cluster := mocks.NewCluster(t)
			orgsClient := mocks.NewClient(t)

			mgr.EXPECT().
				GetCluster(mock.Anything, "root:orgs").
				Return(cluster, nil).
				Maybe()
			cluster.EXPECT().
				GetClient().
				Return(orgsClient).
				Maybe()
			orgsClient.EXPECT().
				Get(mock.Anything, types.NamespacedName{Name: "cluster"}, mock.Anything).
				Run(func(ctx context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) {
					lc := obj.(*kcpcorev1alpha.LogicalCluster)
					lc.Annotations = map[string]string{"kcp.io/cluster": "a"}
				}).
				Return(nil).
				Maybe()

			if test.fgaMocks != nil {
				test.fgaMocks(openfga)
			}

			h := orgs.New(openfga, mgr, "authorization.kubernetes.io/cluster-name", "b")

			ctx := t.Context()

			res := h.Handle(ctx, test.req)
			assert.Equal(t, test.res, res)

		})
	}
}
