package contextual

import (
	"context"
	"fmt"
	"strings"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/clustercache"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

const maxRelationLength = 50

type contextualAuthorizer struct {
	clusterKey   string
	fga          openfgav1.OpenFGAServiceClient
	clusterCache clustercache.Provider
}

var _ authorization.Handler = &contextualAuthorizer{}

func New(fga openfgav1.OpenFGAServiceClient, clusterCache clustercache.Provider, clusterKey string) authorization.Handler {
	return &contextualAuthorizer{
		fga:          fga,
		clusterKey:   clusterKey,
		clusterCache: clusterCache,
	}
}

func (c *contextualAuthorizer) Handle(ctx context.Context, req authorization.Request) authorization.Response {

	klog.V(5).Info("handling request in ContextualAuthorizer")

	if req.Spec.Extra == nil {
		klog.V(5).Info("request does not contain Extra attributes, skipping")
		return authorization.NoOpinion()
	}

	cn, ok := req.Spec.Extra[c.clusterKey]
	if !ok || len(cn) == 0 {
		klog.V(5).Infof("request does not contain expected Extra attribute %q, skipping", c.clusterKey)
		return authorization.NoOpinion()
	}

	clusterName := cn[0]

	klog.V(5).InfoS("found cluster name", "clusterName", clusterName)

	if req.Spec.ResourceAttributes == nil {
		if req.Spec.NonResourceAttributes == nil {
			klog.V(5).Info("request does not contain ResourceAttributes or NonResourceAttributes, skipping")
			return authorization.NoOpinion()
		}

		if !strings.HasPrefix(req.Spec.NonResourceAttributes.Path, "/clusters/") {
			klog.V(5).Info("non-resource request is not cluster-scoped, skipping")
			return authorization.NoOpinion()
		}

		accountInfoCluster, err := c.mgr.GetCluster(ctx, clusterName)
		if err != nil {
			klog.ErrorS(err, "failed to get cluster from manager", "clusterName", clusterName)
			return authorization.Denied()
		}

		var info accounts1alpha1.AccountInfo
		err = accountInfoCluster.GetClient().Get(ctx, types.NamespacedName{Name: "account"}, &info)
		if err != nil {
			klog.ErrorS(err, "failed to get AccountInfo from cluster", "clusterName", clusterName)
			return authorization.Denied()
		}

		if req.Spec.User == "" {
			klog.V(5).Info("request does not contain a user, denying")
			return authorization.Denied()
		}

		accountObject := fmt.Sprintf("core_platform-mesh_io_account:%s/%s", info.Spec.Account.OriginClusterId, info.Spec.Account.Name)
		check := &openfgav1.CheckRequest{
			StoreId: info.Spec.FGA.Store.Id,
			TupleKey: &openfgav1.CheckRequestTupleKey{
				Object:   accountObject,
				Relation: "member",
				User:     fmt.Sprintf("user:%s", req.Spec.User),
			},
		}

		response, err := c.fga.Check(ctx, check)
		if err != nil {
			klog.ErrorS(err, "failed to perform OpenFGA check")
			return authorization.Denied()
		}

		klog.V(5).InfoS("performed OpenFGA membership check", "allowed", response.Allowed)
		if response.Allowed {
			return authorization.Allowed()
		}

		return authorization.Denied()
	}

	clusterInfo, ok := c.clusterCache.Get(clusterName)
	if !ok {
		klog.V(5).InfoS("cluster not found in cache, skipping", "clusterName", clusterName)
		return authorization.NoOpinion()
	}

	klog.V(5).InfoS("found cluster info in cache",
		"clusterName", clusterName,
		"storeID", clusterInfo.StoreID,
		"accountName", clusterInfo.AccountName,
		"parentClusterID", clusterInfo.ParentClusterID)

	attrs := req.Spec.ResourceAttributes

	version := attrs.Version
	if version == "*" {
		// For some cluster level resources, the version may be set to "*". In that case, we should treat it as empty string to avoid issues with RESTMapper.
		version = ""
	}

	gvr := schema.GroupVersionResource{
		Group:    attrs.Group,
		Version:  version,
		Resource: attrs.Resource,
	}

	gvk, err := clusterInfo.RESTMapper.KindFor(gvr)
	if err != nil {
		klog.ErrorS(err, "failed to get GVK for GVR", "GVR", gvr)
		return authorization.NoOpinion()
	}

	klog.V(5).InfoS("mapped GVR to GVK", "GVK", gvk)

	isNamespaced, err := apiutil.IsGVKNamespaced(gvk, clusterInfo.RESTMapper)
	if err != nil {
		klog.ErrorS(err, "failed to determine if GVK is namespaced", "GVK", gvk)
		return authorization.NoOpinion()
	}

	singular, err := clusterInfo.RESTMapper.ResourceSingularizer(attrs.Resource)
	if err != nil {
		klog.ErrorS(err, "failed to singularize resource", "resource", attrs.Resource)
		return authorization.NoOpinion()
	}

	group := util.CapGroupToRelationLength(gvr, maxRelationLength)
	group = strings.ReplaceAll(group, ".", "_")

	objectType := fmt.Sprintf("%s_%s", group, singular)
	longestObjectType := fmt.Sprintf("create_%ss", objectType)
	if len(longestObjectType) > maxRelationLength {
		objectType = objectType[len(longestObjectType)-maxRelationLength:]
	}

	object := fmt.Sprintf("%s:%s/%s", objectType, clusterName, attrs.Name)
	relation := attrs.Verb

	hasParent := util.ResolveOnParent(attrs.Verb)

	accountObject := fmt.Sprintf("core_platform-mesh_io_account:%s/%s", clusterInfo.ParentClusterID, clusterInfo.AccountName)

	if hasParent {
		relation = fmt.Sprintf("%s_%s_%s", relation, group, gvr.Resource)
		object = accountObject
	}

	var contextualTuples []*openfgav1.TupleKey
	if isNamespaced {
		namespaceObject := fmt.Sprintf("core_namespace:%s/%s", clusterName, attrs.Namespace)

		// parent the namespace to the account
		contextualTuples = append(contextualTuples, &openfgav1.TupleKey{
			Object:   namespaceObject,
			Relation: "parent",
			User:     accountObject,
		})

		if hasParent {
			object = namespaceObject
		} else {
			// parent the object to the namespace
			contextualTuples = append(contextualTuples, &openfgav1.TupleKey{
				Object:   object,
				Relation: "parent",
				User:     namespaceObject,
			})
		}
	} else {
		contextualTuples = append(contextualTuples, &openfgav1.TupleKey{
			Object:   fmt.Sprintf("%s:%s/%s", objectType, clusterName, attrs.Name),
			Relation: "parent",
			User:     accountObject,
		})
	}

	klog.InfoS("calling fga", "object", object, "relation", relation)

	check := &openfgav1.CheckRequest{
		StoreId: clusterInfo.StoreID,
		TupleKey: &openfgav1.CheckRequestTupleKey{
			Object:   object,
			Relation: relation,
			User:     fmt.Sprintf("user:%s", req.Spec.User),
		},
	}

	if contextualTuples != nil {
		check.ContextualTuples = &openfgav1.ContextualTupleKeys{
			TupleKeys: contextualTuples,
		}
	}

	response, err := c.fga.Check(ctx, check)
	if err != nil {
		klog.ErrorS(err, "failed to perform OpenFGA check")
		return authorization.NoOpinion()
	}

	klog.V(5).InfoS("performed OpenFGA check", "allowed", response.Allowed)

	if response.Allowed {
		return authorization.Allowed()
	}

	return authorization.Denied()
}
