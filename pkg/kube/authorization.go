package kube

import (
	"context"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CheckSelfSubjectAccess asks Kubernetes whether the current request identity
// may perform the supplied resource operation. Kubernetes remains the policy
// authority; Kite only uses the result to protect data obtained outside the
// target resource endpoint, such as persisted history or Prometheus samples.
func CheckSelfSubjectAccess(
	ctx context.Context,
	clientSet kubernetes.Interface,
	attributes authorizationv1.ResourceAttributes,
) (allowed bool, reason string, err error) {
	review, err := clientSet.AuthorizationV1().SelfSubjectAccessReviews().Create(
		ctx,
		&authorizationv1.SelfSubjectAccessReview{
			Spec: authorizationv1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &attributes,
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return false, "", err
	}
	return review.Status.Allowed, review.Status.Reason, nil
}
