package authorization

import (
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
)

func Errored(err error) Response {
	return Response{
		SubjectAccessReview: authorizationv1.SubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{
				Allowed:         false,
				Reason:          err.Error(),
				EvaluationError: err.Error(),
			},
		},
	}
}

func NoOpinion() Response {
	return Response{
		SubjectAccessReview: authorizationv1.SubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{
				Allowed: false,
				Reason:  "NoOpinion",
			},
		},
	}
}

// Aborted returns a response that is neither allowed nor denied,
// but signals the union chain to stop evaluating further handlers.
func Aborted() Response {
	return Response{
		SubjectAccessReview: authorizationv1.SubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{
				Allowed: false,
				Reason:  "NoOpinion",
			},
		},
		Abort: true,
	}
}

func Allowed() Response {
	return Response{
		SubjectAccessReview: authorizationv1.SubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{
				Allowed: true,
				Denied:  false,
			},
		},
	}
}

func Denied() Response {
	return Response{
		SubjectAccessReview: authorizationv1.SubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{
				Allowed: false,
				Denied:  true,
			},
		},
	}
}

// Retry makes the apiserver retry the request after a given duration
func Retry(after time.Duration) Response {
	// note: it is important to not set a SubjectAccessReview here for the later
	// set Retry-After header to be resprected by the apiserver.
	return Response{
		RetryAfter: after,
	}
}
