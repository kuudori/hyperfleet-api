package auth

import (
	"context"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// K8sValidatorConfig configures a Kubernetes TokenReview-based token validator.
type K8sValidatorConfig struct {
	IssuerURL string
	Audience  string
}

type k8sTokenReviewValidator struct {
	client    kubernetes.Interface
	issuerURL string
	audience  string
}

var _ TokenValidator = (*k8sTokenReviewValidator)(nil)

// NewK8sTokenReviewValidator creates a validator that delegates token verification
// to the Kubernetes API server via the TokenReview API. Returns an error if
// not running in a Kubernetes cluster.
func NewK8sTokenReviewValidator(cfg K8sValidatorConfig) (TokenValidator, error) {
	restCfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("kubernetes in-cluster config: %w", err)
	}

	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client: %w", err)
	}

	return &k8sTokenReviewValidator{
		issuerURL: cfg.IssuerURL,
		audience:  cfg.Audience,
		client:    client,
	}, nil
}

func (v *k8sTokenReviewValidator) Validate(ctx context.Context, rawToken string) (string, *jwt.Token, error) {
	review := &authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token: rawToken,
		},
	}
	if v.audience != "" {
		review.Spec.Audiences = []string{v.audience}
	}

	result, err := v.client.AuthenticationV1().TokenReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("token review request failed: %w", err)
	}

	if !result.Status.Authenticated {
		msg := "token rejected by kubernetes API server"
		if result.Status.Error != "" {
			msg = fmt.Sprintf("%s: %s", msg, result.Status.Error)
		}
		return "", nil, fmt.Errorf("%s", msg)
	}

	token, _, err := jwt.NewParser().ParseUnverified(rawToken, jwt.MapClaims{})
	if err != nil {
		return "", nil, fmt.Errorf("failed to decode validated token claims: %w", err)
	}
	token.Valid = true

	return result.Status.User.Username, token, nil
}

func (v *k8sTokenReviewValidator) Close() {}
