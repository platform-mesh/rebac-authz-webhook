package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kcp-dev/logicalcluster/v3"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	corev1alpha1 "github.com/platform-mesh/account-operator/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	"github.com/platform-mesh/golang-commons/logger"

	"github.com/platform-mesh/rebac-authz-webhook/pkg/mapperprovider"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/util"
)

type AuthorizationHandler struct {
	fga             openfgav1.OpenFGAServiceClient
	accountInfoName string
	orgStoreID      string
	orgWorkspaceID  string
	mgr             mcmanager.Manager
	mps             *mapperprovider.MapperProviders
}

var (
	openfgaLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "openfga_request_duration_seconds",
		Help:    "A histogram of the gRPC request durations to OpenFGA in seconds.",
		Buckets: prometheus.DefBuckets,
	})
)

const rootOrgName = "tenancy_kcp_io_workspace:orgs"

func NewAuthorizationHandler(fga openfgav1.OpenFGAServiceClient, mgr mcmanager.Manager, accountInfoName string, orgStoreID, orgWorkspaceID string, mps *mapperprovider.MapperProviders) (*AuthorizationHandler, error) {
	return &AuthorizationHandler{
		fga:             fga,
		accountInfoName: accountInfoName,
		mgr:             mgr,
		orgStoreID:      orgStoreID,
		orgWorkspaceID:  orgWorkspaceID,
		mps:             mps,
	}, nil
}

var ErrNoStoreID = errors.New("no store ID found")

func (a *AuthorizationHandler) getAccountInfo(ctx context.Context, sar authorizationv1.SubjectAccessReview) (*corev1alpha1.AccountInfo, error) {
	log := logger.LoadLoggerFromContext(ctx)
	info := &corev1alpha1.AccountInfo{}
	clusterNameAttr, ok := sar.Spec.Extra["authorization.kubernetes.io/cluster-name"]
	if !ok || len(clusterNameAttr) == 0 {
		return nil, errors.New("no cluster name found in the request")
	}
	log.Debug().Str("cluster", clusterNameAttr[0]).Str("accountInfoName", a.accountInfoName).Msg("Looking for AccountInfo")

	cluster, err := a.mgr.GetCluster(ctx, clusterNameAttr[0])
	if err != nil {
		log.Error().Err(err).Str("cluster", clusterNameAttr[0]).Msg("Failed to get cluster")
		return nil, errors.Join(err, ErrNoStoreID)
	}

	if err := cluster.GetClient().Get(ctx, types.NamespacedName{Name: a.accountInfoName}, info); err != nil {
		log.Error().Err(err).Str("cluster", clusterNameAttr[0]).Str("accountInfoName", a.accountInfoName).Msg("Failed to get AccountInfo")
		return nil, errors.Join(err, ErrNoStoreID)
	}

	if info.Spec.FGA.Store.Id == "" {
		log.Error().Msg("AccountInfo found but Store.Id is empty")
		return nil, ErrNoStoreID
	}
	log.Debug().Str("storeId", info.Spec.FGA.Store.Id).Msg("Retrieved Store ID")

	return info, nil
}

func (a *AuthorizationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := logger.LoadLoggerFromContext(r.Context())

	var sar authorizationv1.SubjectAccessReview
	err := json.NewDecoder(r.Body).Decode(&sar)
	if err != nil {
		log.Error().Err(err).Msg("unable to decode the request")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = r.Body.Close()
	if err != nil {
		log.Error().Err(err).Msg("unable to close the request body")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log = log.
		ChildLogger("user", sar.Spec.User).
		ChildLogger("requestID", string(sar.UID))

	// Handle non-resource attributes first (API paths)
	if sar.Spec.ResourceAttributes == nil {
		if sar.Spec.NonResourceAttributes == nil || !strings.HasPrefix(sar.Spec.NonResourceAttributes.Path, "/api") {
			noOpinion(w, sar)
			return
		}

		sar.Status = authorizationv1.SubjectAccessReviewStatus{
			Allowed: true,
		}

		if err := json.NewEncoder(w).Encode(&sar); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		return
	}

	// For resource attributes, we need to get the store ID
	accountInfo, err := a.getAccountInfo(r.Context(), sar)
	if err != nil {
		log.Error().Err(err).Str("user", sar.Spec.User).Msg("error getting store ID from account info")
		noOpinion(w, sar)
		return
	}

	log = log.ChildLogger("resourceAttributes", sar.Spec.ResourceAttributes.String()).
		ChildLogger("group", sar.Spec.ResourceAttributes.Group).
		ChildLogger("resource", sar.Spec.ResourceAttributes.Resource).
		ChildLogger("subresource", sar.Spec.ResourceAttributes.Subresource).
		ChildLogger("verb", sar.Spec.ResourceAttributes.Verb)

	log.Debug().Str("sar", fmt.Sprintf("%+v", sar)).Msg("Received SubjectAccessReview")

	group := util.CapGroupToRelationLength(sar, 50)
	group = strings.ReplaceAll(group, ".", "_")
	relation := sar.Spec.ResourceAttributes.Verb

	var clusterName string
	if sar.Spec.Extra != nil {
		if clusterNames, exists := sar.Spec.Extra["authorization.kubernetes.io/cluster-name"]; exists && len(clusterNames) > 0 {
			clusterName = clusterNames[0]
		}
	}
	log = log.ChildLogger("clusterName", clusterName)

	var namespaced bool
	var gvk schema.GroupVersionKind

	restMapper, ok := a.mps.GetMapper(logicalcluster.Name(clusterName))
	if !ok {
		log.Error().Err(err).Msg("error getting provider")
		noOpinion(w, sar)
		return
	}

	gvr := schema.GroupVersionResource{
		Group:    sar.Spec.ResourceAttributes.Group,
		Resource: sar.Spec.ResourceAttributes.Resource,
		Version:  sar.Spec.ResourceAttributes.Version,
	}
	gvk, err = restMapper.KindFor(gvr)
	if err != nil {
		log.Error().Err(err).Str("gvr", fmt.Sprintf("%+v", gvr)).Msg("error getting GVK")
		noOpinion(w, sar)
		return
	}
	log.Debug().Str("gvr", fmt.Sprintf("%+v", gvr)).Str("gvk", fmt.Sprintf("%+v", gvk)).Msg("Got GVK")

	namespaced, err = apiutil.IsGVKNamespaced(gvk, restMapper)
	if err != nil {
		log.Error().Err(err).Msg("error checking if GVK is namespaced")
		noOpinion(w, sar)
		return
	}

	resourceType := sar.Spec.ResourceAttributes.Resource

	if singularResource, err := restMapper.ResourceSingularizer(sar.Spec.ResourceAttributes.Resource); err == nil {
		resourceType = singularResource
		log.Debug().Str("resource", sar.Spec.ResourceAttributes.Resource).Str("singular", resourceType).Msg("Converted resource to singular form")
	}

	objectType := fmt.Sprintf("%s_%s", group, resourceType)

	longestObjectType := fmt.Sprintf("create_%ss", objectType)
	if len(longestObjectType) > 50 {
		objectType = objectType[len(longestObjectType)-50:]
	}

	objectName := sar.Spec.ResourceAttributes.Name

	var object string
	var contextualTuples []*openfgav1.TupleKey

	if util.ResolveOnParent(sar.Spec.ResourceAttributes.Verb) {
		relation = fmt.Sprintf("%s_%s_%s", sar.Spec.ResourceAttributes.Verb, group, sar.Spec.ResourceAttributes.Resource)
		if namespaced {
			object = fmt.Sprintf("core_namespace:%s/%s", clusterName, sar.Spec.ResourceAttributes.Namespace)
			contextualTuples = append(contextualTuples, &openfgav1.TupleKey{
				Object:   object,
				Relation: "parent",
				User:     fmt.Sprintf("core_platform-mesh_io_account:%s/%s", accountInfo.Spec.Account.OriginClusterId, accountInfo.Spec.Account.Name),
			})
		} else {
			object = fmt.Sprintf("core_platform-mesh_io_account:%s/%s", accountInfo.Spec.Account.OriginClusterId, accountInfo.Spec.Account.Name)
		}
	} else {
		object = fmt.Sprintf("%s:%s/%s", objectType, clusterName, objectName)
		if namespaced {
			namespaceObject := fmt.Sprintf("core_namespace:%s/%s", clusterName, sar.Spec.ResourceAttributes.Namespace)
			contextualTuples = append(contextualTuples, &openfgav1.TupleKey{
				Object:   object,
				Relation: "parent",
				User:     namespaceObject,
			})
			contextualTuples = append(contextualTuples, &openfgav1.TupleKey{
				Object:   namespaceObject,
				Relation: "parent",
				User:     fmt.Sprintf("core_platform-mesh_io_account:%s/%s", accountInfo.Spec.Account.OriginClusterId, accountInfo.Spec.Account.Name),
			})
		} else {
			contextualTuples = append(contextualTuples, &openfgav1.TupleKey{
				Object:   object,
				Relation: "parent",
				User:     fmt.Sprintf("core_platform-mesh_io_account:%s/%s", accountInfo.Spec.Account.OriginClusterId, accountInfo.Spec.Account.Name),
			})
		}
	}

	log.Debug().Str("object", object).Str("relation", relation).Any("contextualTuples", contextualTuples).Msg("check call elements")

	if a.fga == nil {
		log.Warn().Msg("FGA client is nil, returning no opinion")
		noOpinion(w, sar)
		return
	}

	// request to orgs workspace
	if a.orgWorkspaceID == clusterName {
		object = rootOrgName
	}

	preReq := time.Now()
	res, err := a.fga.Check(r.Context(), &openfgav1.CheckRequest{
		StoreId: accountInfo.Spec.FGA.Store.Id,
		TupleKey: &openfgav1.CheckRequestTupleKey{
			Object:   object,
			Relation: relation,
			User:     fmt.Sprintf("user:%s", sar.Spec.User),
		},
		ContextualTuples: &openfgav1.ContextualTupleKeys{
			TupleKeys: contextualTuples,
		},
	})

	openfgaLatency.Observe(time.Since(preReq).Seconds())
	if err != nil {
		log.Error().Err(err).Str("storeId", accountInfo.Spec.FGA.Store.Id).Str("object", object).Str("relation", relation).Str("user", sar.Spec.User).Msg("unable to call upstream openfga")
		noOpinion(w, sar)
		return
	}
	log.Info().Str("allowed", fmt.Sprintf("%t", res.Allowed)).Str("user", sar.Spec.User).Str("object", object).Str("relation", relation).Msg("sar response")
	if !res.Allowed {
		noOpinion(w, sar)
		return
	}

	sar.Status = authorizationv1.SubjectAccessReviewStatus{
		Allowed: res.Allowed,
		Denied:  !res.Allowed,
	}

	if err := json.NewEncoder(w).Encode(&sar); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func noOpinion(w http.ResponseWriter, sar authorizationv1.SubjectAccessReview) {
	sar.Status = authorizationv1.SubjectAccessReviewStatus{
		Allowed: false,
		Reason:  "NoOpinion",
	}
	if err := json.NewEncoder(w).Encode(&sar); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
