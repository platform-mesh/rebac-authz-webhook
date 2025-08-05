package util

import (
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
)

func ResolveOnParent(verb string) bool {
	if verb == "create" || verb == "list" || verb == "watch" { // TODO: improve and consider list watch or individual watch
		return true
	}

	return false
}

func CapGroupToRelationLength(sar authorizationv1.SubjectAccessReview, maxLength int) string {

	maxRelation := fmt.Sprintf("create_%s_%s", sar.Spec.ResourceAttributes.Group, sar.Spec.ResourceAttributes.Resource)

	group := sar.Spec.ResourceAttributes.Group
	if group == "" {
		group = "core"
	}

	if len(maxRelation) > maxLength {
		return group[len(maxRelation)-maxLength:]
	}

	return group
}
