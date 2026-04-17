package util

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func FuzzCapGroupToRelationLength(f *testing.F) {
	f.Add("apps", "v1", "deployments", 50)
	f.Add("apis.kcp.io", "", "apibindings", 50)
	f.Add("", "v1", "pods", 50)
	f.Add("very.long.group.name.example.io", "v1", "verylongresourcenamethatexceedslimits", 50)
	f.Add("a", "", "b", 10)
	f.Add("", "", "", 1)

	f.Fuzz(func(t *testing.T, group, version, resource string, maxLength int) {
		if maxLength < 1 {
			return
		}

		gvr := schema.GroupVersionResource{
			Group:    group,
			Version:  version,
			Resource: resource,
		}

		// Must not panic
		_ = CapGroupToRelationLength(gvr, maxLength)
	})
}

func FuzzResolveOnParent(f *testing.F) {
	f.Add("create")
	f.Add("list")
	f.Add("watch")
	f.Add("get")
	f.Add("delete")
	f.Add("")

	f.Fuzz(func(t *testing.T, verb string) {
		// Must not panic
		_ = ResolveOnParent(verb)
	})
}
