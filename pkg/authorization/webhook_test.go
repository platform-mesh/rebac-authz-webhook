package authorization_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

func TestServeHTTP(t *testing.T) {
	testCases := []struct {
		name               string
		handler            authorization.Handler
		req                func() *http.Request
		responseAssertions func(*testing.T, *http.Response)
	}{
		{
			name: "should fail for nil body",
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/authorize", nil)
			},
			responseAssertions: func(t *testing.T, res *http.Response) {
				var sar v1.SubjectAccessReview
				err := json.NewDecoder(res.Body).Decode(&sar)
				assert.NoError(t, err)

				assert.False(t, sar.Status.Allowed)
				assert.Equal(t, "request body is empty", sar.Status.Reason)
			},
		},
		{
			name: "should fail for no body",
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/authorize", http.NoBody)
			},
			responseAssertions: func(t *testing.T, res *http.Response) {
				var sar v1.SubjectAccessReview
				err := json.NewDecoder(res.Body).Decode(&sar)
				assert.NoError(t, err)

				assert.False(t, sar.Status.Allowed)
				assert.Equal(t, "request body is empty", sar.Status.Reason)
			},
		},
		{
			name: "should fail for wrong content-type",
			req: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/authorize", &bytes.Buffer{})
				req.Header.Set("Content-Type", "wrong")
				return req
			},
			responseAssertions: func(t *testing.T, res *http.Response) {
				var sar v1.SubjectAccessReview
				err := json.NewDecoder(res.Body).Decode(&sar)
				assert.NoError(t, err)

				assert.False(t, sar.Status.Allowed)
				assert.Equal(t, "contentType=wrong, expected application/json", sar.Status.Reason)
			},
		},
		{
			name: "should fail for wrong body content",
			req: func() *http.Request {
				var buffer bytes.Buffer
				buffer.WriteString("{")

				req := httptest.NewRequest(http.MethodPost, "/authorize", &buffer)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			responseAssertions: func(t *testing.T, res *http.Response) {
				var sar v1.SubjectAccessReview
				err := json.NewDecoder(res.Body).Decode(&sar)
				assert.NoError(t, err)

				assert.False(t, sar.Status.Allowed)
				assert.Equal(t, "couldn't get version/kind; json parse error: unexpected end of JSON input", sar.Status.Reason)
			},
		},
		{
			name: "should return the response from the handler with the UID",
			req: func() *http.Request {
				var buffer bytes.Buffer
				sar := v1.SubjectAccessReview{
					ObjectMeta: metav1.ObjectMeta{
						UID: "1234",
					},
				}
				err := json.NewEncoder(&buffer).Encode(sar)
				assert.NoError(t, err)

				req := httptest.NewRequest(http.MethodPost, "/authorize", &buffer)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			handler: authorization.HandlerFunc(func(ctx context.Context, r authorization.Request) authorization.Response {
				return authorization.Allowed()
			}),
			responseAssertions: func(t *testing.T, res *http.Response) {
				var sar v1.SubjectAccessReview
				err := json.NewDecoder(res.Body).Decode(&sar)
				assert.NoError(t, err)

				assert.True(t, sar.Status.Allowed)
				assert.Equal(t, types.UID("1234"), sar.UID)
			},
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			wh := authorization.New(klog.NewKlogr(), test.handler)

			res := httptest.NewRecorder()

			wh.ServeHTTP(res, test.req())

			handledResponse := res.Result()
			if test.responseAssertions != nil {
				test.responseAssertions(t, handledResponse)
			}
		})
	}
}
