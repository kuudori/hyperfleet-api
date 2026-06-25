package auth

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	hferrors "github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// JWTHandlerConfig configures the multi-issuer JWT handler.
type JWTHandlerConfig struct {
	Next        http.Handler
	Validators  map[string]TokenValidator
	PublicPaths []string
}

// NewJWTHandler creates a handler that routes tokens to the correct validator
// based on the iss claim and stores the validated token + resolved identity
// in the request context.
func NewJWTHandler(cfg JWTHandlerConfig) (*JWTHandler, error) {
	publicPatterns := make([]*regexp.Regexp, 0, len(cfg.PublicPaths))
	for _, p := range cfg.PublicPaths {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid public path pattern %q: %w", p, err)
		}
		publicPatterns = append(publicPatterns, re)
	}

	return &JWTHandler{
		validators:     cfg.Validators,
		publicPatterns: publicPatterns,
		next:           cfg.Next,
	}, nil
}

// JWTHandler validates JWT tokens by routing to issuer-specific validators.
// Call Close() during shutdown to release validator resources.
type JWTHandler struct {
	validators     map[string]TokenValidator
	next           http.Handler
	publicPatterns []*regexp.Regexp
}

func (h *JWTHandler) Close() {
	for _, v := range h.validators {
		v.Close()
	}
}

func (h *JWTHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, re := range h.publicPatterns {
		if re.MatchString(r.URL.Path) {
			h.next.ServeHTTP(w, r)
			return
		}
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		handleError(r.Context(), w, r, hferrors.CodeAuthNoCredentials, "missing Authorization header")
		return
	}

	parts := strings.Fields(authHeader)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		handleError(r.Context(), w, r, hferrors.CodeAuthInvalidCredentials, "Authorization header must use Bearer scheme")
		return
	}
	tokenString := parts[1]

	issuer, err := peekIssuer(tokenString)
	if err != nil {
		logger.WithError(r.Context(), err).Warn("failed to extract issuer from token")
		handleError(r.Context(), w, r, hferrors.CodeAuthInvalidCredentials, "unable to determine token issuer")
		return
	}

	validator, ok := h.validators[issuer]
	if !ok {
		msg := fmt.Sprintf("token issuer %q is not recognized", issuer)
		handleError(r.Context(), w, r, hferrors.CodeAuthInvalidCredentials, msg)
		return
	}

	identity, token, err := validator.Validate(r.Context(), tokenString)
	if err != nil {
		logger.WithError(r.Context(), err).Warn("JWT validation failed")
		if errors.Is(err, jwt.ErrTokenExpired) {
			handleError(r.Context(), w, r, hferrors.CodeAuthExpiredToken, "JWT token has expired")
		} else {
			handleError(r.Context(), w, r, hferrors.CodeAuthInvalidCredentials, "invalid or expired JWT token")
		}
		return
	}

	ctx := SetJWTTokenContext(r.Context(), token)
	if identity != "" {
		ctx = SetResolvedIdentityContext(ctx, identity)
	}
	h.next.ServeHTTP(w, r.WithContext(ctx))
}
