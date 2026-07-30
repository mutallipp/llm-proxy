package biz

import (
	"errors"

	"github.com/mutallipp/llm-proxy/llm/transformer"
)

var (
	ErrInvalidJWT             = errors.New("invalid jwt token")
	ErrInvalidToken           = errors.New("invalid token")
	ErrInvalidAPIKey          = errors.New("invalid api key")
	ErrInvalidPassword        = errors.New("invalid password")
	ErrInvalidModel           = transformer.ErrInvalidModel
	ErrAdapterNotFound        = errors.New("adapter not found")
	ErrAdapterAlreadyExists   = errors.New("adapter name already exists")
	ErrAdapterInvalidName     = errors.New("invalid adapter name")
	ErrModelGroupInUse        = errors.New("model group is still referenced by active bindings")
	ErrInternal               = errors.New("server internal error, please try again later")
	ErrAPIKeyOwnerRequired    = errors.New("owner api key is required")
	ErrServiceAccountRequired = errors.New("service account api key required")
	ErrAPIKeyScopeRequired    = errors.New("api key missing required scope")
	ErrAPIKeyNameRequired     = errors.New("api key name is required")
	ErrSystemNotInitialized   = errors.New("system not initialized")
	ErrOIDCLoginRequired      = errors.New("OIDC user without password, please login via OIDC or set a password")
	ErrProjectNotFound        = errors.New("project not found")
)
