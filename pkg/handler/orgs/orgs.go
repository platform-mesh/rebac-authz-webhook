package orgs

import (
	"context"
	"fmt"
	"strings"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/util"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

const rootOrgName = "tenancy_kcp_io_workspace:orgs"

type orgsAuthorizer struct {
	clusterKey      string
	orgsWorkspaceID string
	orgsStoreID     string
	fga             openfgav1.OpenFGAServiceClient
}

var _ authorization.Handler = &orgsAuthorizer{}

func New(fga openfgav1.OpenFGAServiceClient, clusterKey, orgsWorkspaceID, orgsStoreID string) authorization.Handler {
	return &orgsAuthorizer{
		clusterKey:      clusterKey,
		orgsWorkspaceID: orgsWorkspaceID,
		orgsStoreID:     orgsStoreID,
		fga:             fga,
	}
}

func (o *orgsAuthorizer) Handle(ctx context.Context, req authorization.Request) authorization.Response {

	klog.V(5).Info("handling request in OrgsAuthorizer")

	if req.Spec.Extra == nil {
		klog.V(5).Info("request does not contain Extra attributes, skipping")
		return authorization.NoOpinion()
	}

	cn, ok := req.Spec.Extra[o.clusterKey]
	if !ok || len(cn) == 0 {
		klog.V(5).Infof("request does not contain expected Extra attribute %q, skipping", o.clusterKey)
		return authorization.NoOpinion()
	}

	clusterName := cn[0]

	if req.Spec.ResourceAttributes == nil {
		klog.V(5).Info("request does not contain ResourceAttributes, skipping")
		return authorization.NoOpinion()
	}

	if clusterName != o.orgsWorkspaceID {
		klog.V(5).Infof("request cluster name %q does not match org workspace ID %q, skipping", clusterName, o.orgsWorkspaceID)
		return authorization.NoOpinion()
	}

	klog.V(2).Infof("request cluster name %q matches org workspace ID %q, requesting fga", clusterName, o.orgsWorkspaceID)

	attrs := req.Spec.ResourceAttributes

	group := util.CapGroupToRelationLength(schema.GroupVersionResource{Group: attrs.Group, Version: attrs.Version, Resource: attrs.Resource}, 50)
	group = strings.ReplaceAll(group, ".", "_")

	res, err := o.fga.Check(ctx, &openfgav1.CheckRequest{
		StoreId: o.orgsStoreID,
		TupleKey: &openfgav1.CheckRequestTupleKey{
			Object:   rootOrgName,
			Relation: fmt.Sprintf("%s_%s_%s", attrs.Verb, group, attrs.Resource),
			User:     fmt.Sprintf("user:%s", req.Spec.User),
		},
	})
	if err != nil {
		klog.Errorf("error checking fga for user %q in orgs store %q: %v", req.Spec.User, o.orgsStoreID, err)
		return authorization.NoOpinion()
	}

	if res.Allowed {
		return authorization.Allowed()
	}

	return authorization.Aborted()
}
