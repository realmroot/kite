package model

import (
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/realmroot/lightkite/pkg/common"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"k8s.io/klog/v2"
)

var (
	DB *gorm.DB

	once sync.Once
)

type Model struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func InitDB() {
	dsn := common.DBDSN
	level := logger.Silent
	if klog.V(10).Enabled() {
		level = logger.Info
	}
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold: time.Second,
			LogLevel:      level,
			Colorful:      false,
		},
	)

	var err error
	once.Do(func() {
		cfg := &gorm.Config{
			Logger: newLogger,
		}
		if common.DBType == "sqlite" {
			DB, err = gorm.Open(sqlite.Open(dsn), cfg)
			if err != nil {
				panic("failed to connect database: " + err.Error())
			}
		}

		if common.DBType == "mysql" {
			mysqlDSN := strings.TrimPrefix(dsn, "mysql://")
			if !strings.Contains(mysqlDSN, "parseTime=") {
				separator := "?"
				if strings.Contains(mysqlDSN, "?") {
					separator = "&"
				}
				mysqlDSN = mysqlDSN + separator + "parseTime=true"
			}
			DB, err = gorm.Open(mysql.Open(mysqlDSN), cfg)
			if err != nil {
				panic("failed to connect database: " + err.Error())
			}
		}

		if common.DBType == "postgres" {
			DB, err = gorm.Open(postgres.Open(dsn), cfg)
			if err != nil {
				panic("failed to connect database: " + err.Error())
			}
		}
	})

	if DB == nil {
		panic("database connection is nil, check your DB_TYPE and DB_DSN settings")
	}
	sqldb, err := DB.DB()
	if err != nil {
		panic("failed to configure database pool: " + err.Error())
	}
	if common.DBType == "sqlite" {
		// Lightkite's SQLite database stores low-volume product metadata. A single
		// connection keeps transaction locking and connection-local PRAGMAs
		// deterministic; multi-replica deployments use PostgreSQL or MySQL.
		sqldb.SetMaxOpenConns(1)
		sqldb.SetMaxIdleConns(1)
	} else {
		sqldb.SetMaxOpenConns(common.DBMaxOpenConns)
		sqldb.SetMaxIdleConns(common.DBMaxIdleConns)
	}
	sqldb.SetConnMaxLifetime(common.DBMaxIdleTime)

	// For SQLite we must enable foreign key enforcement explicitly.
	// SQLite has foreign key constraints defined in the schema but they are
	// not enforced unless PRAGMA foreign_keys = ON is set on the connection.
	if common.DBType == "sqlite" {
		if err := DB.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
			panic("failed to enable sqlite foreign keys: " + err.Error())
		}
	}
	if err := runSchemaMigrations(DB); err != nil {
		panic("failed to run database migrations: " + err.Error())
	}
	models := []interface{}{
		User{},
		Cluster{},
		GeneralSetting{},
		ResourceHistory{},
		ResourceTemplate{},
		OIDCSession{},
		HelmRepository{},
	}
	for _, model := range models {
		err = DB.AutoMigrate(model)
		if err != nil {
			panic("failed to migrate database: " + err.Error())
		}
	}
}
