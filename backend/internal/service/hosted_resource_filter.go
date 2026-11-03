package service

import (
	"context"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type hostedResourceFilterKey struct{}

// WithHostedResourceFilter scopes admin inventory queries only. Internal queries
// without this filter continue to include hosted resources for scheduling.
func WithHostedResourceFilter(ctx context.Context, hosted bool) context.Context {
	return context.WithValue(ctx, hostedResourceFilterKey{}, hosted)
}

func HostedResourceFilterFromContext(ctx context.Context) (bool, bool) {
	hosted, ok := ctx.Value(hostedResourceFilterKey{}).(bool)
	return hosted, ok
}

func WithAccountOwnershipFilter(ctx context.Context, ownership string) (context.Context, error) {
	switch strings.TrimSpace(ownership) {
	case "", "standard":
		return WithHostedResourceFilter(ctx, false), nil
	case "hosted":
		return WithHostedResourceFilter(ctx, true), nil
	case "all":
		return ctx, nil
	default:
		return nil, infraerrors.BadRequest("INVALID_ACCOUNT_OWNERSHIP", "invalid account ownership filter")
	}
}
