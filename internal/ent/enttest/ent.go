package enttest

import (
	"database/sql"

	"entgo.io/ent/dialect/sql/schema"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/migrate"
	"github.com/mutallipp/llm-proxy/internal/ent/migrate/schemahook"
	_ "github.com/mutallipp/llm-proxy/internal/pkg/sqlite"
)

func NewEntClient(t TestingT, driverName, dataSourceName string) *ent.Client {
	sqlDB, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		panic(err)
	}

	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	return NewClient(t,
		WithOptions(
			ent.Driver(entsql.OpenDB(driverName, sqlDB)),
		),
		WithMigrateOptions(
			migrate.WithGlobalUniqueID(false),
			migrate.WithForeignKeys(false),
			migrate.WithDropIndex(true),
			migrate.WithDropColumn(true),
			schema.WithHooks(schemahook.V0_3_0),
		),
	)
}
