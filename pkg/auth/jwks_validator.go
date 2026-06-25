package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

const (
	defaultSigningAlgorithm = "RS256"
	defaultLeeway           = 30 * time.Second
)

// JWKSValidatorConfig configures a JWKS-based token validator for a single issuer.
type JWKSValidatorConfig struct {
	IssuerURL         string
	Audience          string
	IdentityClaimName string
	KeysFile          string
	KeysURL           string
}

type jwksValidator struct {
	keyfunc       keyfunc.Keyfunc
	parser        *jwt.Parser
	cancel        context.CancelFunc
	issuerURL     string
	identityClaim string
}

var _ TokenValidator = (*jwksValidator)(nil)

func NewJWKSValidator(ctx context.Context, cfg JWKSValidatorConfig) (TokenValidator, error) {
	ctx, cancel := context.WithCancel(ctx)

	kf, err := buildKeyfunc(ctx, cfg.KeysFile, cfg.KeysURL)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to build JWKS keyfunc: %w", err)
	}

	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{defaultSigningAlgorithm}),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(defaultLeeway),
	}
	if cfg.IssuerURL != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(cfg.IssuerURL))
	} else {
		logger.Warn(ctx, "JWT issuer validation disabled: no issuer_url configured")
	}
	if cfg.Audience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(cfg.Audience))
	}

	return &jwksValidator{
		issuerURL:     cfg.IssuerURL,
		identityClaim: cfg.IdentityClaimName,
		keyfunc:       kf,
		parser:        jwt.NewParser(parserOpts...),
		cancel:        cancel,
	}, nil
}

func (v *jwksValidator) Validate(_ context.Context, rawToken string) (string, *jwt.Token, error) {
	token, err := v.parser.Parse(rawToken, v.keyfunc.Keyfunc)
	if err != nil {
		return "", nil, err
	}

	var identity string
	if claims, ok := token.Claims.(jwt.MapClaims); ok && v.identityClaim != "" {
		identity, ok = claims[v.identityClaim].(string)
		if !ok || identity == "" {
			return "", nil, fmt.Errorf("token missing required identity claim %q", v.identityClaim)
		}
	}

	return identity, token, nil
}

func (v *jwksValidator) Close() {
	if v.cancel != nil {
		v.cancel()
	}
}

func buildKeyfunc(ctx context.Context, keysFile, keysURL string) (keyfunc.Keyfunc, error) {
	hasFile := keysFile != ""
	hasURL := keysURL != ""

	if !hasFile && !hasURL {
		return nil, fmt.Errorf("at least one of KeysFile or KeysURL must be provided")
	}

	if hasFile && !hasURL {
		data, err := os.ReadFile(keysFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read JWKS file %q: %w", keysFile, err)
		}
		kf, err := keyfunc.NewJWKSetJSON(json.RawMessage(data))
		if err != nil {
			return nil, fmt.Errorf("failed to parse JWKS file %q: %w", keysFile, err)
		}
		return kf, nil
	}

	if !hasFile && hasURL {
		kf, err := keyfunc.NewDefaultCtx(ctx, []string{keysURL})
		if err != nil {
			return nil, fmt.Errorf("failed to create JWKS client from URL %q: %w", keysURL, err)
		}
		return kf, nil
	}

	data, err := os.ReadFile(keysFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWKS file %q: %w", keysFile, err)
	}
	fileKF, err := keyfunc.NewJWKSetJSON(json.RawMessage(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWKS file: %w", err)
	}

	httpStorage, err := jwkset.NewHTTPClient(jwkset.HTTPClientOptions{
		Given: fileKF.Storage(),
		HTTPURLs: map[string]jwkset.Storage{
			keysURL: jwkset.NewMemoryStorage(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP JWKS client: %w", err)
	}

	kf, err := keyfunc.New(keyfunc.Options{
		Ctx:     ctx,
		Storage: httpStorage,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create combined JWKS keyfunc: %w", err)
	}
	return kf, nil
}
