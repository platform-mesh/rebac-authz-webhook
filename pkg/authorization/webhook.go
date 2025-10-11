package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-logr/logr"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var authorizationScheme = runtime.NewScheme()
var authorizationCodecs = serializer.NewCodecFactory(authorizationScheme)

func init() {
	utilruntime.Must(authorizationv1.AddToScheme(authorizationScheme))
}

var _ http.Handler = &Webhook{}

// Handler can handle an SubjectAccessReview.
type Handler interface {
	// Handle yields a response to an SubjectAccessReview.
	//
	// The supplied context is extracted from the received http.Request, allowing wrapping
	// http.Handlers to inject values into and control cancelation of downstream request processing.
	Handle(context.Context, Request) Response
}

// HandlerFunc implements Handler interface using a single function.
type HandlerFunc func(context.Context, Request) Response

var _ Handler = HandlerFunc(nil)

// Handle process the TokenReview by invoking the underlying function.
func (f HandlerFunc) Handle(ctx context.Context, req Request) Response {
	return f(ctx, req)
}

// Webhook represents each individual webhook.
type Webhook struct {
	// Handler actually processes an authorization request returning whether it was authorized or unauthorized.
	Handler Handler

	log logr.Logger
}

func New(log logr.Logger, handler Handler) *Webhook {
	return &Webhook{
		Handler: handler,
		log:     log.WithName("webhook"),
	}
}

// Request defines the input for an authorization handler.
type Request struct {
	authorizationv1.SubjectAccessReview
}

// Response is the output of an authorization handler.
// It contains a response indicating if a given
// operation is allowed.
type Response struct {
	authorizationv1.SubjectAccessReview
	Abort bool `json:"-"`
}

// ServeHTTP implements http.Handler.
func (wh *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Body == nil || r.Body == http.NoBody {
		err := errors.New("request body is empty")
		wh.log.Error(err, "empty request body")
		wh.writeResponse(w, Errored(err))
		return
	}

	defer r.Body.Close()

	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		err := fmt.Errorf("contentType=%s, expected application/json", contentType)
		wh.log.Error(err, "invalid content type")
		wh.writeResponse(w, Errored(err))
		return
	}

	body, err := io.ReadAll(r.Body) // TODO: one could size of usnig a os.LimitReader to not parse infinitly large bodies
	if err != nil {
		wh.log.Error(err, "unable to read the body from the incoming request")
		wh.writeResponse(w, Errored(err))
		return
	}

	var sar authorizationv1.SubjectAccessReview
	_, _, err = authorizationCodecs.UniversalDecoder().Decode(body, nil, &sar)
	if err != nil {
		wh.log.Error(err, "unable to decode the request")
		wh.writeResponse(w, Errored(err))
		return
	}

	// TODO: think of log constructor
	wh.log.V(5).Info("received request")

	req := Request{sar}

	res := wh.Handler.Handle(ctx, req)
	res.UID = req.UID

	wh.writeResponse(w, res)
}

func (wh *Webhook) writeResponse(w io.Writer, resp Response) {
	if err := json.NewEncoder(w).Encode(resp.SubjectAccessReview); err != nil {
		wh.log.Error(err, "unable to encode the response")
		wh.writeResponse(w, Errored(err))
	}

	wh.log.V(5).Info("wrote response", "requestID", resp.UID, "authorized", resp.Status.Allowed)
}
