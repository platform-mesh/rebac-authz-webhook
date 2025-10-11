package util

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func ResolveOnParent(verb string) bool {
	return verb == "create" || verb == "list" || verb == "watch"
}

func CapGroupToRelationLength(gvr schema.GroupVersionResource, maxLength int) string {

	maxRelation := fmt.Sprintf("create_%s_%s", gvr.Group, gvr.Resource)

	group := gvr.Group
	if group == "" {
		group = "core"
	}

	if len(maxRelation) > maxLength {
		return group[len(maxRelation)-maxLength:]
	}

	return group
}
