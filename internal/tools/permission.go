package tools

import "context"

type PermissionRequester interface {
	RequestPermission(ctx context.Context, toolName, path, resolvedPath, operation string, isDangerous bool) (bool, error)
}
