package model

import "gorm.io/gorm"

type Cluster struct {
	Model
	Name                   string       `json:"name" gorm:"type:varchar(100);uniqueIndex;not null"`
	Description            string       `json:"description" gorm:"type:text"`
	APIServerURL           string       `json:"apiServerUrl" gorm:"type:text"`
	CABundle               string       `json:"caBundle,omitempty" gorm:"type:text"`
	TLSServerName          string       `json:"tlsServerName,omitempty" gorm:"type:varchar(255)"`
	ConnectionMode         string       `json:"connectionMode" gorm:"type:varchar(20);default:direct"`
	PrometheusURL          string       `json:"prometheus_url,omitempty" gorm:"type:text"`
	InCluster              bool         `json:"in_cluster" gorm:"type:boolean;default:false"`
	ClusterAgent           bool         `json:"clusterAgent" gorm:"type:boolean;default:false"`
	ClusterAgentTokenHash  string       `json:"-" gorm:"type:varchar(64);index"`
	ClusterAgentPublicKey  string       `json:"-" gorm:"type:varchar(64)"`
	ClusterAgentPrivateKey SecretString `json:"-" gorm:"type:text"`
	IsDefault              bool         `json:"is_default" gorm:"type:boolean;default:false"`
	Enable                 bool         `json:"enable" gorm:"type:boolean;not null"`
}

func AddCluster(cluster *Cluster) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if cluster.IsDefault {
			if err := tx.Model(&Cluster{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(cluster).Error
	})
}

func GetClusterByName(name string) (*Cluster, error) {
	var cluster Cluster
	if err := DB.Where("name = ?", name).First(&cluster).Error; err != nil {
		return nil, err
	}
	return &cluster, nil
}

func GetClusterByID(id uint) (*Cluster, error) {
	var cluster Cluster
	if err := DB.First(&cluster, id).Error; err != nil {
		return nil, err
	}
	return &cluster, nil
}

func GetClusterByClusterAgentTokenHash(hash string) (*Cluster, error) {
	var cluster Cluster
	if err := DB.Where("cluster_agent_token_hash = ?", hash).First(&cluster).Error; err != nil {
		return nil, err
	}
	return &cluster, nil
}

func UpdateCluster(cluster *Cluster, updates map[string]interface{}) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if isDefault, ok := updates["is_default"].(bool); ok && isDefault {
			if err := tx.Model(&Cluster{}).Where("id <> ? AND is_default = ?", cluster.ID, true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(cluster).Updates(updates).Error; err != nil {
			return err
		}
		return nil
	})
}

func DeleteCluster(cluster *Cluster) error {
	return DB.Unscoped().Delete(cluster).Error
}

func ListClusters() ([]*Cluster, error) {
	var clusters []*Cluster
	if err := DB.Find(&clusters).Error; err != nil {
		return nil, err
	}
	return clusters, nil
}
