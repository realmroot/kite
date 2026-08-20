package ai

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	pkgmodel "github.com/zxh326/kite/pkg/model"
)

func currentUserFromGin(c *gin.Context) (pkgmodel.User, bool) {
	rawUser, ok := c.Get("user")
	if !ok {
		return pkgmodel.User{}, false
	}
	user, ok := rawUser.(pkgmodel.User)
	return user, ok
}

func AuthorizeTool(c *gin.Context, cs *cluster.ClientSet, toolName string, args map[string]interface{}) (string, bool) {
	if c == nil {
		return "Error: authorization context is required", true
	}
	if cs == nil {
		return "Error: cluster client is required", true
	}
	if _, ok := currentUserFromGin(c); !ok {
		return "Error: authenticated user not found in context", true
	}
	return "", false
}

// ExecuteTool runs a tool and returns the result as a string.
func ExecuteTool(ctx context.Context, c *gin.Context, cs *cluster.ClientSet, toolName string, args map[string]interface{}) (string, bool) {
	if result, isError := AuthorizeTool(c, cs, toolName, args); isError {
		return result, true
	}

	user, _ := currentUserFromGin(c)

	switch toolName {
	case "get_resource":
		return executeGetResource(ctx, cs, args)
	case "describe_resource":
		return executeDescribeResource(ctx, cs, args)
	case "list_resources":
		return executeListResources(ctx, cs, args)
	case "get_pod_logs":
		return executeGetPodLogs(ctx, cs, args)
	case "exec_in_pod":
		return executeExecInPod(ctx, cs, args)
	case "get_cluster_overview":
		return executeGetClusterOverview(ctx, cs)
	case "create_resource":
		return executeCreateResource(ctx, cs, user, args)
	case "apply_resource":
		return executeApplyResource(ctx, cs, user, args)
	case "update_resource":
		return executeUpdateResource(ctx, cs, user, args)
	case "patch_resource":
		return executePatchResource(ctx, cs, user, args)
	case "delete_resource":
		return executeDeleteResource(ctx, cs, user, args)
	case "query_prometheus":
		return executeQueryPrometheus(ctx, cs, args)
	default:
		return fmt.Sprintf("Unknown tool: %s", toolName), true
	}
}
