package scheduler

import (
	"context"

	"github.com/zxh326/kite/pkg/cluster"
)

func Start(ctx context.Context, cm *cluster.ClusterManager, tokens IdentityTokenProvider) {
	manager := NewManager()

	registerHelmReleaseAutoUpgradeExecutor(manager, cm, tokens)
	manager.Start(ctx)
}
