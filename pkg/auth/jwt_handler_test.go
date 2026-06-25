package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mendsley/gojwk"
	. "github.com/onsi/gomega"
)

const testKID = "test-key-1"
const testIssuer = "https://test-issuer.example.com"

func newTestHandler(
	t *testing.T, validators map[string]TokenValidator, next http.Handler, publicPaths ...string,
) *JWTHandler {
	t.Helper()
	h, err := NewJWTHandler(JWTHandlerConfig{
		Validators:  validators,
		PublicPaths: publicPaths,
		Next:        next,
	})
	if err != nil {
		t.Fatalf("failed to create JWTHandler: %v", err)
	}
	return h
}

func validatorMap(validators ...TokenValidator) map[string]TokenValidator {
	m := make(map[string]TokenValidator, len(validators))
	for _, v := range validators {
		switch vt := v.(type) {
		case *jwksValidator:
			m[vt.issuerURL] = v
		default:
			panic("unsupported validator type in test helper")
		}
	}
	return m
}

func newJWKSTestValidator(t *testing.T, keysURL, issuerURL string, opts ...func(*JWKSValidatorConfig)) TokenValidator {
	t.Helper()
	cfg := JWKSValidatorConfig{
		KeysURL:   keysURL,
		IssuerURL: issuerURL,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	v, err := NewJWKSValidator(t.Context(), cfg)
	if err != nil {
		t.Fatalf("failed to create JWKSValidator: %v", err)
	}
	t.Cleanup(v.Close)
	return v
}

func TestJWTHandler(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer := newJWKSServer(t, &privateKey.PublicKey)
	defer jwksServer.Close()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	validator := newJWKSTestValidator(t, jwksServer.URL, testIssuer)
	handler := newTestHandler(t, validatorMap(validator), nextHandler, "^/healthz$", "^/openapi$")

	t.Run("valid token passes through", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": testIssuer,
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(Equal("ok"))
	})

	t.Run("valid token sets claims in context", func(t *testing.T) {
		RegisterTestingT(t)
		claimsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := GetJWTTokenFromContext(r.Context())
			Expect(tok).NotTo(BeNil())
			claims, ok := tok.Claims.(jwt.MapClaims)
			Expect(ok).To(BeTrue())
			Expect(claims["username"]).To(Equal("testuser"))
			w.WriteHeader(http.StatusOK)
		})
		h := newTestHandler(t, validatorMap(validator), claimsHandler)

		token := signToken(t, privateKey, jwt.MapClaims{
			"iss":      testIssuer,
			"exp":      time.Now().Add(time.Hour).Unix(),
			"iat":      time.Now().Unix(),
			"username": "testuser",
		})
		rr := serve(h, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
	})

	t.Run("expired token returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": testIssuer,
			"exp": time.Now().Add(-time.Hour).Unix(),
			"iat": time.Now().Add(-2 * time.Hour).Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("invalid signature returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
		Expect(err).NotTo(HaveOccurred())
		token := signToken(t, otherKey, jwt.MapClaims{
			"iss": testIssuer,
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("wrong issuer returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://wrong-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("missing Authorization header returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(handler, "/protected", "")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("lowercase bearer scheme accepted per RFC 7235", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": testIssuer,
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(handler, "/protected", "bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
	})

	t.Run("malformed Authorization header returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(handler, "/protected", "Basic dXNlcjpwYXNz")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("garbage token returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(handler, "/protected", "Bearer not.a.jwt")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("public endpoint without token passes through", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(handler, "/healthz", "")
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(Equal("ok"))
	})

	t.Run("public endpoint with invalid token still passes through", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(handler, "/healthz", "Bearer garbage")
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(Equal("ok"))
	})

	t.Run("HS256 signed token rejected", func(t *testing.T) {
		RegisterTestingT(t)
		claims := jwt.MapClaims{
			"iss": testIssuer,
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, err := tok.SignedString([]byte("secret-key-for-hmac"))
		Expect(err).NotTo(HaveOccurred())
		rr := serve(handler, "/protected", "Bearer "+tokenString)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})
}

func TestJWTHandler_MultiIssuer(t *testing.T) {
	RegisterTestingT(t)

	key1, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())
	key2, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer1 := newJWKSServer(t, &key1.PublicKey)
	defer jwksServer1.Close()
	jwksServer2 := newJWKSServer(t, &key2.PublicKey)
	defer jwksServer2.Close()

	issuer1 := "https://issuer-one.example.com"
	issuer2 := "https://issuer-two.example.com"

	v1 := newJWKSTestValidator(t, jwksServer1.URL, issuer1, func(c *JWKSValidatorConfig) {
		c.IdentityClaimName = "email"
	})
	v2 := newJWKSTestValidator(t, jwksServer2.URL, issuer2, func(c *JWKSValidatorConfig) {
		c.IdentityClaimName = "sub"
	})

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity := GetResolvedIdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, identity)
	})

	handler := newTestHandler(t, validatorMap(v1, v2), nextHandler)

	t.Run("routes to first issuer and resolves email", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, key1, jwt.MapClaims{
			"iss":   issuer1,
			"email": "user@example.com",
			"exp":   time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(Equal("user@example.com"))
	})

	t.Run("routes to second issuer and resolves sub", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, key2, jwt.MapClaims{
			"iss": issuer2,
			"sub": "system:serviceaccount:ns:sa",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(Equal("system:serviceaccount:ns:sa"))
	})

	t.Run("unknown issuer returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, key1, jwt.MapClaims{
			"iss": "https://unknown.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("token signed by wrong key for matched issuer returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, key2, jwt.MapClaims{
			"iss": issuer1,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})
}

func TestJWTHandler_FailClosed_NoValidKeys(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badServer.Close()

	validator := newJWKSTestValidator(t, badServer.URL, testIssuer)
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := newTestHandler(t, validatorMap(validator), okHandler)

	token := signToken(t, privateKey, jwt.MapClaims{
		"iss": testIssuer,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	rr := serve(handler, "/protected", "Bearer "+token)
	Expect(rr.Code).To(Equal(http.StatusUnauthorized))
}

func TestJWTHandler_WithAudience(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer := newJWKSServer(t, &privateKey.PublicKey)
	defer jwksServer.Close()

	validator := newJWKSTestValidator(t, jwksServer.URL, testIssuer, func(c *JWKSValidatorConfig) {
		c.Audience = "my-api"
	})
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := newTestHandler(t, validatorMap(validator), okHandler)

	t.Run("correct audience passes", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": testIssuer,
			"aud": "my-api",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
	})

	t.Run("wrong audience returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": testIssuer,
			"aud": "wrong-api",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})
}

func TestJWTHandler_WithoutAudience_AcceptsAny(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer := newJWKSServer(t, &privateKey.PublicKey)
	defer jwksServer.Close()

	validator := newJWKSTestValidator(t, jwksServer.URL, testIssuer)
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := newTestHandler(t, validatorMap(validator), okHandler)

	token := signToken(t, privateKey, jwt.MapClaims{
		"iss": testIssuer,
		"aud": "any-audience",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	rr := serve(handler, "/protected", "Bearer "+token)
	Expect(rr.Code).To(Equal(http.StatusOK))
}

func TestJWTHandler_FileOnlyKeyfunc(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksFile := writeJWKSFile(t, &privateKey.PublicKey)

	v, err := NewJWKSValidator(t.Context(), JWKSValidatorConfig{
		KeysFile:  jwksFile,
		IssuerURL: testIssuer,
	})
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(v.Close)

	handler := newTestHandler(t, validatorMap(v), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))

	t.Run("valid token accepted via file keys", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": testIssuer,
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(Equal("ok"))
	})

	t.Run("wrong key rejected via file keys", func(t *testing.T) {
		RegisterTestingT(t)
		otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
		Expect(err).NotTo(HaveOccurred())
		token := signToken(t, otherKey, jwt.MapClaims{
			"iss": testIssuer,
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})
}

func TestJWTHandler_CombinedKeyfunc(t *testing.T) {
	RegisterTestingT(t)

	fileKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksFile := writeJWKSFile(t, &fileKey.PublicKey)
	jwksServer := newJWKSServer(t, &fileKey.PublicKey)
	defer jwksServer.Close()

	v, err := NewJWKSValidator(t.Context(), JWKSValidatorConfig{
		KeysFile:  jwksFile,
		KeysURL:   jwksServer.URL,
		IssuerURL: testIssuer,
	})
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(v.Close)

	handler := newTestHandler(t, validatorMap(v), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("constructor succeeds with both file and URL", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(handler).NotTo(BeNil())
	})

	t.Run("file key accepted in combined mode", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, fileKey, jwt.MapClaims{
			"iss": testIssuer,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
	})

	t.Run("unknown key rejected in combined mode", func(t *testing.T) {
		RegisterTestingT(t)
		otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
		Expect(err).NotTo(HaveOccurred())
		token := signToken(t, otherKey, jwt.MapClaims{
			"iss": testIssuer,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})
}

func TestJWTHandler_Close(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer := newJWKSServer(t, &privateKey.PublicKey)
	defer jwksServer.Close()

	v, err := NewJWKSValidator(t.Context(), JWKSValidatorConfig{
		KeysURL:   jwksServer.URL,
		IssuerURL: testIssuer,
	})
	Expect(err).NotTo(HaveOccurred())

	handler := newTestHandler(t, validatorMap(v), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	handler.Close()
	handler.Close() // idempotent, should not panic
}

func TestJWTHandler_ResponseBody(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer := newJWKSServer(t, &privateKey.PublicKey)
	defer jwksServer.Close()

	validator := newJWKSTestValidator(t, jwksServer.URL, testIssuer)
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := newTestHandler(t, validatorMap(validator), okHandler)

	t.Run("missing header returns problem+json with no-credentials code", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(handler, "/protected", "")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))

		var body map[string]any
		Expect(json.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
		Expect(body["code"]).To(Equal("HYPERFLEET-AUT-001"))
		Expect(body["status"]).To(BeNumerically("==", 401))
	})

	t.Run("expired token returns problem+json with expired code", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": testIssuer,
			"exp": time.Now().Add(-time.Hour).Unix(),
			"iat": time.Now().Add(-2 * time.Hour).Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))

		var body map[string]any
		Expect(json.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
		Expect(body["code"]).To(Equal("HYPERFLEET-AUT-003"))
		Expect(body["status"]).To(BeNumerically("==", 401))
	})

	t.Run("invalid token returns problem+json with invalid-credentials code", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(handler, "/protected", "Bearer not.a.jwt")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))

		var body map[string]any
		Expect(json.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
		Expect(body["code"]).To(Equal("HYPERFLEET-AUT-002"))
		Expect(body["status"]).To(BeNumerically("==", 401))
	})

	t.Run("non-Bearer scheme returns problem+json with invalid-credentials code", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(handler, "/protected", "Basic dXNlcjpwYXNz")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))

		var body map[string]any
		Expect(json.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
		Expect(body["code"]).To(Equal("HYPERFLEET-AUT-002"))
		Expect(body["status"]).To(BeNumerically("==", 401))
	})
}

func TestJWKSValidator_RequiresKeysConfig(t *testing.T) {
	RegisterTestingT(t)

	_, err := NewJWKSValidator(context.Background(), JWKSValidatorConfig{
		IssuerURL: testIssuer,
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("KeysFile or KeysURL"))
}

func TestJWKSValidator_RejectsMissingIdentityClaim(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer := newJWKSServer(t, &privateKey.PublicKey)
	defer jwksServer.Close()

	v := newJWKSTestValidator(t, jwksServer.URL, testIssuer, func(c *JWKSValidatorConfig) {
		c.IdentityClaimName = "email"
	})

	token := signToken(t, privateKey, jwt.MapClaims{
		"iss": testIssuer,
		"sub": "client-credentials-grant",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	_, _, err = v.Validate(context.Background(), token)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("missing required identity claim"))
	Expect(err.Error()).To(ContainSubstring("email"))
}

// --- helpers ---

func signToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testKID
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return s
}

func serve(handler http.Handler, path, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func writeJWKSFile(t *testing.T, pubKey *rsa.PublicKey) string {
	t.Helper()
	jwk, err := gojwk.PublicKey(pubKey)
	if err != nil {
		t.Fatalf("failed to create JWK: %v", err)
	}
	jwk.Kid = testKID
	jwk.Alg = "RS256"
	jwkBytes, err := gojwk.Marshal(jwk)
	if err != nil {
		t.Fatalf("failed to marshal JWK: %v", err)
	}
	data := fmt.Sprintf(`{"keys":[%s]}`, string(jwkBytes))
	path := filepath.Join(t.TempDir(), "jwks.json")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatalf("failed to write JWKS file: %v", err)
	}
	return path
}

func newJWKSServer(t *testing.T, pubKey *rsa.PublicKey) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwk, err := gojwk.PublicKey(pubKey)
		if err != nil {
			t.Errorf("failed to create JWK: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		jwk.Kid = testKID
		jwk.Alg = "RS256"
		jwkBytes, err := gojwk.Marshal(jwk)
		if err != nil {
			t.Errorf("failed to marshal JWK: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"keys":[%s]}`, string(jwkBytes))
	}))
}
