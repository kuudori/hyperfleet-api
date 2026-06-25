package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type apiServer struct {
	httpServer *http.Server
	jwtHandler *auth.JWTHandler
}

var _ Server = &apiServer{}

func env() *environments.Env {
	return environments.Environment()
}

func NewAPIServer(tracingEnabled bool) Server {
	s := &apiServer{}

	mainRouter := s.routes(tracingEnabled)

	// referring to the router as type http.Handler allows us to add middleware via more handlers
	var mainHandler http.Handler = mainRouter

	if env().Config.Server.JWT.Enabled {
		ctx := context.Background()
		validators, err := buildTokenValidators(ctx)
		check(err, "Unable to create token validators")

		jwtHandler, err := auth.NewJWTHandler(auth.JWTHandlerConfig{
			Validators: validators,
			PublicPaths: []string{
				"^/api/hyperfleet/?$",
				"^/api/hyperfleet/v1/?$",
				"^/api/hyperfleet/v1/openapi/?$",
				"^/api/hyperfleet/v1/openapi.html/?$",
				"^/api/hyperfleet/v1/errors(/.*)?$",
			},
			Next: mainHandler,
		})
		check(err, "Unable to create JWT authentication handler")
		s.jwtHandler = jwtHandler
		mainHandler = jwtHandler
	}

	mainHandler = removeTrailingSlash(mainHandler)

	s.httpServer = &http.Server{
		Addr:              env().Config.Server.BindAddress(),
		Handler:           mainHandler,
		ReadTimeout:       env().Config.Server.Timeouts.Read,
		WriteTimeout:      env().Config.Server.Timeouts.Write,
		ReadHeaderTimeout: 10 * time.Second, // Hardcoded to prevent Slowloris attacks (not user-configurable)
	}

	return s
}

// Serve start the blocking call to Serve.
// Useful for breaking up ListenAndServer (Start) when you require the server to be listening before continuing
func (s apiServer) Serve(listener net.Listener) {
	ctx := context.Background()
	var err error
	if env().Config.Server.TLS.Enabled {
		// Check https cert and key path
		if env().Config.Server.TLS.CertFile == "" || env().Config.Server.TLS.KeyFile == "" {
			check(
				fmt.Errorf(
					"HTTPS certificate or key not configured; "+
						"set via server.tls.cert_file/key_file in config file, env vars, or flags",
				),
				"Can't start https server",
			)
		}

		// Serve with TLS
		logger.With(ctx, logger.FieldBindAddress, env().Config.Server.BindAddress()).Info("Serving with TLS")
		err = s.httpServer.ServeTLS(listener, env().Config.Server.TLS.CertFile, env().Config.Server.TLS.KeyFile)
	} else {
		logger.With(ctx, logger.FieldBindAddress, env().Config.Server.BindAddress()).Info("Serving without TLS")
		err = s.httpServer.Serve(listener)
	}

	// Web server terminated.
	if err != nil && err != http.ErrServerClosed {
		check(err, "Web server terminated with errors")
	} else {
		logger.Info(ctx, "Web server terminated")
	}
}

// Listen only start the listener, not the server.
// Useful for breaking up ListenAndServer (Start) when you require the server to be listening before continuing
func (s apiServer) Listen() (listener net.Listener, err error) {
	return net.Listen("tcp", env().Config.Server.BindAddress())
}

// Start listening on the configured port and start the server.
// This is a convenience wrapper for Listen() and Serve(listener Listener)
func (s apiServer) Start() {
	ctx := context.Background()
	listener, err := s.Listen()
	if err != nil {
		logger.WithError(ctx, err).Error("Unable to start API server")
		os.Exit(1)
	}
	s.Serve(listener)

	// after the server exits but before the application terminates
	// we need to explicitly close Go's sql connection pool.
	// this needs to be called *exactly* once during an app's lifetime.
	if err := env().Database.SessionFactory.Close(); err != nil {
		logger.WithError(ctx, err).Error("Error closing database connection")
	}
}

func buildTokenValidators(ctx context.Context) (map[string]auth.TokenValidator, error) {
	cfg := env().Config.Server
	issuers := cfg.JWT.ResolvedIssuers(cfg.JWK)

	validators := make(map[string]auth.TokenValidator, len(issuers))
	for _, ic := range issuers {
		switch ic.Type {
		case config.IssuerTypeJWKS:
			v, err := auth.NewJWKSValidator(ctx, auth.JWKSValidatorConfig{
				IssuerURL:         ic.IssuerURL,
				Audience:          ic.Audience,
				IdentityClaimName: ic.IdentityClaim,
				KeysFile:          ic.JWKCertFile,
				KeysURL:           ic.JWKCertURL,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create JWKS validator for issuer %q: %w", ic.IssuerURL, err)
			}
			logger.With(ctx, "issuer", ic.IssuerURL).Info("Registered JWKS token validator")
			validators[ic.IssuerURL] = v

		case config.IssuerTypeK8sTokenReview:
			v, err := auth.NewK8sTokenReviewValidator(auth.K8sValidatorConfig{
				IssuerURL: ic.IssuerURL,
				Audience:  ic.Audience,
			})
			if err != nil {
				if ic.Optional {
					logger.With(ctx, "issuer", ic.IssuerURL).WithError(err).Warn("k8s-token-review issuer skipped (optional: true)")
					continue
				}
				return nil, fmt.Errorf(
					"k8s-token-review validator for issuer %q failed (set optional: true to skip outside cluster): %w",
					ic.IssuerURL, err,
				)
			}
			logger.With(ctx, "issuer", ic.IssuerURL).Info("Registered k8s TokenReview validator")
			validators[ic.IssuerURL] = v

		default:
			return nil, fmt.Errorf("unknown issuer type %q for issuer %q", ic.Type, ic.IssuerURL)
		}
	}

	if len(validators) == 0 {
		return nil, fmt.Errorf("jwt is enabled but no token validators could be created")
	}

	return validators, nil
}

func (s apiServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := s.httpServer.Shutdown(ctx)
	// Close JWT handler after HTTP drain so in-flight requests can still verify tokens.
	if s.jwtHandler != nil {
		s.jwtHandler.Close()
	}
	return err
}
