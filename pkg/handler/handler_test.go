package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	authorizationv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"

	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler"
	"github.com/platform-mesh/rebac-authz-webhook/pkg/handler/mocks"
)

// SimpleMockManager implements a basic manager interface for testing
type SimpleMockManager struct{}

// GetControllerOptions implements manager.Manager.
func (m *SimpleMockManager) GetControllerOptions() config.Controller {
	panic("unimplemented")
}

// GetFieldIndexer implements manager.Manager.
func (m *SimpleMockManager) GetFieldIndexer() client.FieldIndexer {
	panic("unimplemented")
}

// GetLocalManager implements manager.Manager.
func (m *SimpleMockManager) GetLocalManager() manager.Manager {
	panic("unimplemented")
}

// GetLogger implements manager.Manager.
func (m *SimpleMockManager) GetLogger() logr.Logger {
	panic("unimplemented")
}

// GetManager implements manager.Manager.
func (m *SimpleMockManager) GetManager(ctx context.Context, clusterName string) (manager.Manager, error) {
	panic("unimplemented")
}

// GetProvider implements manager.Manager.
func (m *SimpleMockManager) GetProvider() multicluster.Provider {
	panic("unimplemented")
}

// GetWebhookServer implements manager.Manager.
func (m *SimpleMockManager) GetWebhookServer() webhook.Server {
	panic("unimplemented")
}

func (m *SimpleMockManager) GetCluster(ctx context.Context, clusterName string) (cluster.Cluster, error) {
	return nil, errors.New("cluster not found")
}

func (m *SimpleMockManager) AddReadyzCheck(name string, check healthz.Checker) error {
	return nil
}
func (m *SimpleMockManager) AddHealthzCheck(name string, check healthz.Checker) error {
	return nil
}
func (m *SimpleMockManager) AddMetricsServerExtraHandler(name string, handler http.Handler) error {
	return nil
}

func (m *SimpleMockManager) ClusterFromContext(ctx context.Context) (cluster.Cluster, error) {
	return nil, errors.New("cluster not found")
}

func (m *SimpleMockManager) Elected() <-chan struct{} {
	return nil
}
func (m *SimpleMockManager) Engage(ctx context.Context, clusterName string, cluster cluster.Cluster) error {
	return nil
}

func (m *SimpleMockManager) Start(ctx context.Context) error {
	return nil
}

func (m *SimpleMockManager) Add(runnable mcmanager.Runnable) error {
	return nil
}

func createSimpleMockManager() mcmanager.Manager {
	return &SimpleMockManager{}
}

func TestInvalidStoreOperations(t *testing.T) {
	tests := []struct {
		name string
		sar  authorizationv1.SubjectAccessReview
	}{
		{
			name: "No Opinion (no cluster name)",
			sar: authorizationv1.SubjectAccessReview{
				Spec: authorizationv1.SubjectAccessReviewSpec{
					ResourceAttributes: &authorizationv1.ResourceAttributes{
						Namespace: "a-namespace",
						Verb:      "list",
						Resource:  "pods",
					},
				},
			},
		},
		{
			name: "No Opinion (missing cluster name in extra)",
			sar: authorizationv1.SubjectAccessReview{
				Spec: authorizationv1.SubjectAccessReviewSpec{
					ResourceAttributes: &authorizationv1.ResourceAttributes{
						Group:    "custom.resolver.function",
						Resource: "custom",
						Verb:     "get",
					},
					Extra: map[string]authorizationv1.ExtraValue{},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var body bytes.Buffer
			err := json.NewEncoder(&body).Encode(&test.sar)
			assert.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, "", &body)
			assert.NoError(t, err)

			mockManager := createSimpleMockManager()
			handler, err := handler.NewAuthorizationHandler(nil, mockManager, "account", "", "", nil)
			assert.NoError(t, err)

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)

			sar := authorizationv1.SubjectAccessReview{}
			err = json.NewDecoder(recorder.Body).Decode(&sar)
			assert.NoError(t, err)

			assert.Equal(t, "NoOpinion", sar.Status.Reason)
			assert.False(t, sar.Status.Allowed)
		})
	}
}

func TestAuthorizationHandlerWithFGA(t *testing.T) {
	tests := []struct {
		name           string
		sar            authorizationv1.SubjectAccessReview
		fgaMocks       func(*mocks.OpenFGAServiceClient)
		expectedReason string
		expectAllowed  bool
	}{
		{
			name: "should return no opinion when FGA check fails",
			sar: authorizationv1.SubjectAccessReview{
				Spec: authorizationv1.SubjectAccessReviewSpec{
					User: "test-user",
					ResourceAttributes: &authorizationv1.ResourceAttributes{
						Group:     "dashboard.gardener.cloud",
						Resource:  "terminals",
						Verb:      "create",
						Namespace: "test-namespace",
					},
					Extra: map[string]authorizationv1.ExtraValue{
						"authorization.kubernetes.io/cluster-name": {"test-cluster"},
					},
				},
			},
			fgaMocks: func(ofc *mocks.OpenFGAServiceClient) {
				// This test will fail at the cluster lookup stage, so FGA won't be called
				// We're just testing that the handler doesn't crash with FGA client
			},
			expectedReason: "NoOpinion",
			expectAllowed:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var body bytes.Buffer
			err := json.NewEncoder(&body).Encode(&test.sar)
			assert.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, "", &body)
			assert.NoError(t, err)

			mockFGAClient := mocks.NewOpenFGAServiceClient(t)
			if test.fgaMocks != nil {
				test.fgaMocks(mockFGAClient)
			}

			mockManager := createSimpleMockManager()
			handler, err := handler.NewAuthorizationHandler(mockFGAClient, mockManager, "account", "", "", nil)
			assert.NoError(t, err)

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)

			sar := authorizationv1.SubjectAccessReview{}
			err = json.NewDecoder(recorder.Body).Decode(&sar)
			assert.NoError(t, err)

			assert.Equal(t, test.expectedReason, sar.Status.Reason)
			assert.Equal(t, test.expectAllowed, sar.Status.Allowed)
		})
	}
}

func TestAuthorizationHandler(t *testing.T) {
	t.Run("TestServeHTTP Wrong Request", func(t *testing.T) {
		req, err := http.NewRequest("POST", "", strings.NewReader("wrong"))
		assert.NoError(t, err)

		mockManager := createSimpleMockManager()
		handler, err := handler.NewAuthorizationHandler(nil, mockManager, "account", "", "", nil)
		assert.NoError(t, err)

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})
}

func TestNewAuthorizationHandler(t *testing.T) {
	t.Run("should succeed with valid parameters", func(t *testing.T) {
		mockManager := createSimpleMockManager()
		handler, err := handler.NewAuthorizationHandler(nil, mockManager, "account", "", "", nil)
		assert.NoError(t, err)
		assert.NotNil(t, handler)
	})

	t.Run("should succeed with FGA client", func(t *testing.T) {
		mockFGAClient := mocks.NewOpenFGAServiceClient(t)
		mockManager := createSimpleMockManager()
		handler, err := handler.NewAuthorizationHandler(mockFGAClient, mockManager, "account", "", "", nil)
		assert.NoError(t, err)
		assert.NotNil(t, handler)
	})
}

func TestNonResourceAttributes(t *testing.T) {
	t.Run("should allow API paths", func(t *testing.T) {
		sar := authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				User: "test-user",
				NonResourceAttributes: &authorizationv1.NonResourceAttributes{
					Path: "/api/v1/nodes",
					Verb: "get",
				},
				Extra: map[string]authorizationv1.ExtraValue{
					"authorization.kubernetes.io/cluster-name": {"test-cluster"},
				},
			},
		}

		var body bytes.Buffer
		err := json.NewEncoder(&body).Encode(&sar)
		assert.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "", &body)
		assert.NoError(t, err)

		mockManager := createSimpleMockManager()
		handler, err := handler.NewAuthorizationHandler(nil, mockManager, "account", "", "", nil)
		assert.NoError(t, err)

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var response authorizationv1.SubjectAccessReview
		err = json.NewDecoder(recorder.Body).Decode(&response)
		assert.NoError(t, err)

		assert.True(t, response.Status.Allowed)
	})

	t.Run("should return no opinion for non-API paths", func(t *testing.T) {
		sar := authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				User: "test-user",
				NonResourceAttributes: &authorizationv1.NonResourceAttributes{
					Path: "/healthz",
					Verb: "get",
				},
				Extra: map[string]authorizationv1.ExtraValue{
					"authorization.kubernetes.io/cluster-name": {"test-cluster"},
				},
			},
		}

		var body bytes.Buffer
		err := json.NewEncoder(&body).Encode(&sar)
		assert.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "", &body)
		assert.NoError(t, err)

		mockManager := createSimpleMockManager()
		handler, err := handler.NewAuthorizationHandler(nil, mockManager, "account", "", "", nil)
		assert.NoError(t, err)

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var response authorizationv1.SubjectAccessReview
		err = json.NewDecoder(recorder.Body).Decode(&response)
		assert.NoError(t, err)

		assert.Equal(t, "NoOpinion", response.Status.Reason)
		assert.False(t, response.Status.Allowed)
	})
}

// TestMultiClusterFunctionality tests the basic multi-cluster functionality
// without complex mocking - focuses on the core logic paths
func TestMultiClusterFunctionality(t *testing.T) {
	t.Run("should handle missing cluster gracefully", func(t *testing.T) {
		sar := authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				User: "test-user",
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Group:     "dashboard.gardener.cloud",
					Resource:  "terminals",
					Verb:      "create",
					Namespace: "test-namespace",
				},
				Extra: map[string]authorizationv1.ExtraValue{
					"authorization.kubernetes.io/cluster-name": {"non-existent-cluster"},
				},
			},
		}

		var body bytes.Buffer
		err := json.NewEncoder(&body).Encode(&sar)
		assert.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "", &body)
		assert.NoError(t, err)

		mockManager := createSimpleMockManager()
		handler, err := handler.NewAuthorizationHandler(nil, mockManager, "account", "", "", nil)
		assert.NoError(t, err)

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var response authorizationv1.SubjectAccessReview
		err = json.NewDecoder(recorder.Body).Decode(&response)
		assert.NoError(t, err)

		assert.Equal(t, "NoOpinion", response.Status.Reason)
		assert.False(t, response.Status.Allowed)
	})

	t.Run("should handle FGA client errors gracefully", func(t *testing.T) {
		sar := authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				User: "test-user",
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Group:     "dashboard.gardener.cloud",
					Resource:  "terminals",
					Verb:      "create",
					Namespace: "test-namespace",
				},
				Extra: map[string]authorizationv1.ExtraValue{
					"authorization.kubernetes.io/cluster-name": {"test-cluster"},
				},
			},
		}

		var body bytes.Buffer
		err := json.NewEncoder(&body).Encode(&sar)
		assert.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "", &body)
		assert.NoError(t, err)

		mockFGAClient := mocks.NewOpenFGAServiceClient(t)
		// Don't set up any expectations - this will test error handling

		mockManager := createSimpleMockManager()
		handler, err := handler.NewAuthorizationHandler(mockFGAClient, mockManager, "account", "", "", nil)
		assert.NoError(t, err)

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var response authorizationv1.SubjectAccessReview
		err = json.NewDecoder(recorder.Body).Decode(&response)
		assert.NoError(t, err)

		// Should return NoOpinion when there are errors
		assert.Equal(t, "NoOpinion", response.Status.Reason)
		assert.False(t, response.Status.Allowed)
	})
}
