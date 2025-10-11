package contextual_test

import (
	"testing"

	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/contextual"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/mocks"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/authorization/v1"
)

func TestHandler(t *testing.T) {
	testCases := []struct {
		name                string
		allowedPathPrefixes []string
		req                 authorization.Request
		res                 authorization.Response
		fgaMocks            func(openfga *mocks.OpenFGAServiceClient)
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
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {

			openfga := mocks.NewOpenFGAServiceClient(t)
			if test.fgaMocks != nil {
				test.fgaMocks(openfga)
			}

			h := contextual.New(nil, openfga, "authorization.kubernetes.io/cluster-name")

			ctx := t.Context()

			res := h.Handle(ctx, test.req)

			assert.Equal(t, test.res, res)
		})
	}
}
