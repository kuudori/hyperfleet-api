package auth

import (
	"context"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// TokenValidator validates bearer tokens from a specific issuer.
// Validate returns the resolved caller identity and the parsed token.
// The identity is extracted from the issuer-specific claim (e.g. "email" for
// JWKS, "sub" / TokenReview username for k8s SA tokens).
type TokenValidator interface {
	Validate(ctx context.Context, rawToken string) (identity string, token *jwt.Token, err error)
	Close()
}

// peekIssuer extracts the "iss" claim from a JWT without verifying the signature.
// Used to route tokens to the correct validator before validation.
func peekIssuer(rawToken string) (string, error) {
	token, _, err := jwt.NewParser().ParseUnverified(rawToken, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("failed to parse token: %w", err)
	}

	issuer, err := token.Claims.GetIssuer()
	if err != nil || issuer == "" {
		return "", fmt.Errorf("token has no iss claim")
	}

	return issuer, nil
}
