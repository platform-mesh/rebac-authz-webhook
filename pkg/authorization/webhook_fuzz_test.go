package authorization_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"

	"k8s.io/klog/v2"
)

func FuzzWebhookServeHTTP(f *testing.F) {
	f.Add([]byte(`{"apiVersion":"authorization.k8s.io/v1","kind":"SubjectAccessReview","spec":{"user":"test","resourceAttributes":{"verb":"get","resource":"pods"}}}`))
	f.Add([]byte(`{"apiVersion":"authorization.k8s.io/v1beta1","kind":"SubjectAccessReview","spec":{"user":"test"}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`not json`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		handler := authorization.HandlerFunc(func(_ context.Context, _ authorization.Request) authorization.Response {
			return authorization.Allowed()
		})
		wh := authorization.New(klog.NewKlogr(), handler)

		req := httptest.NewRequest(http.MethodPost, "/authorize", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		wh.ServeHTTP(rec, req)
	})
}
