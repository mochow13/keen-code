package tools

import "context"

type PermissionRequester interface {
	RequestPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous bool) (bool, error)
}
