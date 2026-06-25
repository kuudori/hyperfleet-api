package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/gomega"
	authv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const k8sTestIssuer = "https://kubernetes.default.svc"

func fakeK8sJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".fakesig"
}

func newFakeK8sValidator(t *testing.T, client *fake.Clientset) *k8sTokenReviewValidator {
	t.Helper()
	return &k8sTokenReviewValidator{
		issuerURL: k8sTestIssuer,
		audience:  "hyperfleet-api",
		client:    client,
	}
}

func TestK8sTokenReviewValidator_Validate_Success(t *testing.T) {
	RegisterTestingT(t)

	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authv1.TokenReview)
		review.Status = authv1.TokenReviewStatus{
			Authenticated: true,
			User: authv1.UserInfo{
				Username: "system:serviceaccount:hyperfleet:my-adapter",
				UID:      "abc-123",
				Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:hyperfleet"},
			},
		}
		return true, review, nil
	})

	v := newFakeK8sValidator(t, client)

	rawToken := fakeK8sJWT(t, map[string]any{
		"iss": k8sTestIssuer,
		"sub": "system:serviceaccount:hyperfleet:my-adapter",
		"aud": []string{"hyperfleet-api"},
	})

	identity, token, err := v.Validate(context.Background(), rawToken)
	Expect(err).NotTo(HaveOccurred())
	Expect(identity).To(Equal("system:serviceaccount:hyperfleet:my-adapter"))
	Expect(token).NotTo(BeNil())
	Expect(token.Valid).To(BeTrue())

	claims, ok := token.Claims.(jwt.MapClaims)
	Expect(ok).To(BeTrue())
	Expect(claims["iss"]).To(Equal(k8sTestIssuer))
}

func TestK8sTokenReviewValidator_Validate_Rejected(t *testing.T) {
	RegisterTestingT(t)

	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authv1.TokenReview)
		review.Status = authv1.TokenReviewStatus{
			Authenticated: false,
			Error:         "token has expired",
		}
		return true, review, nil
	})

	v := newFakeK8sValidator(t, client)

	rawToken := fakeK8sJWT(t, map[string]any{
		"iss": k8sTestIssuer,
		"sub": "system:serviceaccount:hyperfleet:my-adapter",
	})

	_, _, err := v.Validate(context.Background(), rawToken)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("token rejected"))
	Expect(err.Error()).To(ContainSubstring("token has expired"))
}

func TestK8sTokenReviewValidator_Validate_APIError(t *testing.T) {
	RegisterTestingT(t)

	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(fmt.Errorf("connection refused"))
	})

	v := newFakeK8sValidator(t, client)

	rawToken := fakeK8sJWT(t, map[string]any{
		"iss": k8sTestIssuer,
		"sub": "test",
	})

	_, _, err := v.Validate(context.Background(), rawToken)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("token review request failed"))
}

func TestK8sTokenReviewValidator_Validate_ReturnsUsername(t *testing.T) {
	RegisterTestingT(t)

	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authv1.TokenReview)
		review.Status = authv1.TokenReviewStatus{
			Authenticated: true,
			User: authv1.UserInfo{
				Username: "system:serviceaccount:ns:sa",
			},
		}
		return true, review, nil
	})

	v := newFakeK8sValidator(t, client)

	rawToken := fakeK8sJWT(t, map[string]any{
		"iss": k8sTestIssuer,
	})

	identity, _, err := v.Validate(context.Background(), rawToken)
	Expect(err).NotTo(HaveOccurred())
	Expect(identity).To(Equal("system:serviceaccount:ns:sa"))
}

func TestK8sTokenReviewValidator_Close(t *testing.T) {
	v := &k8sTokenReviewValidator{}
	v.Close()
}
