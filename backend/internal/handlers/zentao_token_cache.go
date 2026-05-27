package handlers

import (
	"context"
	"zenmind/internal/zentaoauth"
)

// deleteAPIToken clears the cached zentao API token (called after 401 or unbind).
func deleteAPIToken(ctx context.Context, sub string) {
	zentaoauth.DeleteAPIToken(ctx, sub)
}

// loginAndCacheAPIToken fetches credentials and obtains a new API token.
func loginAndCacheAPIToken(ctx context.Context, sub string) (string, error) {
	return zentaoauth.LoginAndCacheAPIToken(ctx, sub)
}

// ensureAPIToken returns the cached token or obtains a fresh one.
func ensureAPIToken(ctx context.Context, sub string) (string, error) {
	return zentaoauth.EnsureAPIToken(ctx, sub)
}
