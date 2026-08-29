package resources

import (
	"github.com/realmroot/lightkite/pkg/cluster"
	"gorm.io/gorm"
)

func scopeResourceHistoryCluster(query *gorm.DB, clientSet *cluster.ClientSet) *gorm.DB {
	if clientSet.ClusterID != 0 {
		return query.Where("cluster_id = ?", clientSet.ClusterID)
	}
	return query.Where("cluster_name = ?", clientSet.Name)
}
