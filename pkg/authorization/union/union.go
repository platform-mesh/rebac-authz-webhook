package union

import (
	"context"

	"github.com/platform-mesh/rebac-authz-webhook/pkg/authorization"
)

type authorizationUnion struct {
	Handlers []authorization.Handler
}

// Handle implements authorization.Handler.
func (u *authorizationUnion) Handle(ctx context.Context, req authorization.Request) authorization.Response {
	for _, h := range u.Handlers {
		resp := h.Handle(ctx, req)

		// if there is an explicit response from one of the handlers, return it
		if resp.Status.Allowed || resp.Status.Denied || resp.Abort {
			return resp
		}
	}

	return authorization.NoOpinion()
}

var _ authorization.Handler = &authorizationUnion{}

func New(requestHandlers ...authorization.Handler) authorization.Handler {
	if len(requestHandlers) == 1 {
		return requestHandlers[0]
	}

	return &authorizationUnion{
		Handlers: requestHandlers,
	}
}
