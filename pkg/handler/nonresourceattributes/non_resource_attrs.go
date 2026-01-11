package nonresourceattributes

import (
	"context"
	"strings"

	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"

	"k8s.io/klog/v2"
)

type nonResourceAttributesAuthorizer struct {
	allowedPathPrefixes []string
}

var _ authorization.Handler = &nonResourceAttributesAuthorizer{}

func New(allowedPathPrefixes ...string) authorization.Handler {
	return &nonResourceAttributesAuthorizer{
		allowedPathPrefixes,
	}
}

func (n *nonResourceAttributesAuthorizer) Handle(ctx context.Context, req authorization.Request) authorization.Response {

	klog.V(5).Info("handling request in NonResourceAttributesAuthorizer")

	if req.Spec.NonResourceAttributes == nil {
		klog.V(5).Info("request does not contain NonResourceAttributes, skipping")
		return authorization.NoOpinion()
	}

	attrs := req.Spec.NonResourceAttributes

	for _, prefix := range n.allowedPathPrefixes {
		if strings.HasPrefix(attrs.Path, prefix) {
			klog.V(5).Infof("request path %q matches allowed prefix %q, allowing", attrs.Path, prefix)
			return authorization.Allowed()
		}
	}

	return authorization.Aborted()
}
