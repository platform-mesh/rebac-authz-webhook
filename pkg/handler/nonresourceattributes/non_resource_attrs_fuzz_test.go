package nonresourceattributes_test

import (
	"testing"

	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/nonresourceattributes"

	v1 "k8s.io/api/authorization/v1"
)

func FuzzNonResourceAttributes(f *testing.F) {
	f.Add("/healthz", "/healthz,/readyz,/api")
	f.Add("/api/v1/namespaces", "/api")
	f.Add("/metrics", "/healthz,/readyz")
	f.Add("", "/healthz")
	f.Add("/", "")
	f.Add("/healthz/ready", "/healthz")

	f.Fuzz(func(t *testing.T, path, prefixesCSV string) {
		var prefixes []string
		if prefixesCSV != "" {
			prefixes = splitCSV(prefixesCSV)
		}

		handler := nonresourceattributes.New(prefixes...)
		req := authorization.Request{
			SubjectAccessReview: v1.SubjectAccessReview{
				Spec: v1.SubjectAccessReviewSpec{
					NonResourceAttributes: &v1.NonResourceAttributes{
						Path: path,
					},
				},
			},
		}

		// Must not panic
		handler.Handle(t.Context(), req)
	})
}

func splitCSV(s string) []string {
	var result []string
	start := 0
	for i := range len(s) {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}
