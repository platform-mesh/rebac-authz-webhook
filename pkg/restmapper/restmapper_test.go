package restmapper_test

import (
	"testing"

	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/mocks"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/restmapper"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"
)

// this test does not really verify if the url was parsed correctly, since i do not know
// how to access the private fields of the RESTMapper created.
func TestEngage(t *testing.T) {
	testCases := []struct {
		name        string
		clusterName string
		cfg         *rest.Config
	}{
		{
			name:        "should replace url properly",
			clusterName: "abcde",
			cfg: &rest.Config{
				Host: "https://example.com/services/apiexports/some-export/",
			},
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			rm := restmapper.New()
			cl := mocks.NewCluster(t)

			cl.EXPECT().GetConfig().Return(test.cfg)

			ctx := t.Context()

			err := rm.Engage(ctx, test.clusterName, cl)
			assert.NoError(t, err)

		})
	}
}
