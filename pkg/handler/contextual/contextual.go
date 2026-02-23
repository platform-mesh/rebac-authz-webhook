package contextual

import (
	"context"
	"errors"
	"fmt"
	"strings"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	accounts1alpha1 "github.com/platform-mesh/account-operator/api/v1alpha1"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/restmapper"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

const maxRelationLength = 50

type contextualAuthorizer struct {
	clusterKey     string
	mgr            mcmanager.Manager
	fga            openfgav1.OpenFGAServiceClient
	mapperProvider restmapper.Provider
}

var _ authorization.Handler = &contextualAuthorizer{}

func New(mgr mcmanager.Manager, fga openfgav1.OpenFGAServiceClient, mapperProvider restmapper.Provider, clusterKey string) authorization.Handler {
	return &contextualAuthorizer{
		mgr:            mgr,
		fga:            fga,
		clusterKey:     clusterKey,
		mapperProvider: mapperProvider,
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

	accountInfoCluster, err := c.mgr.GetCluster(ctx, clusterName)
	if err != nil {
		klog.ErrorS(err, "failed to get cluster from manager", "clusterName", clusterName)
		return authorization.NoOpinion()
	}

	var info accounts1alpha1.AccountInfo
	err = accountInfoCluster.GetClient().Get(ctx, types.NamespacedName{Name: "account"}, &info)
	if err != nil {
		klog.ErrorS(err, "failed to get AccountInfo from cluster", "clusterName", clusterName)
		return authorization.NoOpinion()
	}

	klog.V(5).InfoS("fetched AccountInfo from cluster")

	attrs := req.Spec.ResourceAttributes

	version := attrs.Version
	if version == "*" {
		version = ""
	}

	gvr := schema.GroupVersionResource{
		Group:    attrs.Group,
		Version:  version,
		Resource: attrs.Resource,
	}

	restMapper, ok := c.mapperProvider.Get(clusterName)
	if !ok {
		klog.ErrorS(errors.New("failed to get RESTMapper for cluster"), "clusterName", clusterName)
		return authorization.NoOpinion()
	}

	gvk, err := restMapper.KindFor(gvr)
	if err != nil {
		klog.ErrorS(err, "failed to get GVK for GVR", "GVR", gvr)
		return authorization.NoOpinion()
	}

	klog.V(5).InfoS("mapped GVR to GVK", "GVK", gvk)

	isNamespaced, err := apiutil.IsGVKNamespaced(gvk, restMapper)
	if err != nil {
		klog.ErrorS(err, "failed to determine if GVK is namespaced", "GVK", gvk)
		return authorization.NoOpinion()
	}

	singular, err := restMapper.ResourceSingularizer(attrs.Resource)
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

	accountObject := fmt.Sprintf("core_platform-mesh_io_account:%s/%s", info.Spec.Account.OriginClusterId, info.Spec.Account.Name)

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
		StoreId: info.Spec.FGA.Store.Id,
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
