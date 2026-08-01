package datamigrate

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/Masterminds/semver/v3"

	"github.com/mutallipp/llm-proxy/internal/ent"
)

var legacyModelGroupTables = []string{"model_group_protocols", "model_group_targets", "model_groups"}

// DropLegacyModelGroupTables 仅在 beta7 handoff 已由系统版本标记确认后删除旧表。
// MySQL 的 DDL 可能隐式提交，因此这里先完成全部安全检查，再逐表执行 DROP。
func DropLegacyModelGroupTables(ctx context.Context, client *ent.Client) error {
	drv := ent.RawDriver(client)
	existing, err := existingLegacyTables(ctx, drv)
	if err != nil || len(existing) == 0 {
		return err
	}

	version, err := (&systemVersionReader{client: client}).version(ctx)
	if err != nil {
		return err
	}
	marked, err := semver.NewVersion(version)
	if err != nil || marked.LessThan(mustVersion("v1.0.0-beta7")) {
		return fmt.Errorf("refusing to drop legacy model-group tables: beta7 handoff marker missing (system version %q)", version)
	}

	for _, table := range existing {
		query := fmt.Sprintf("DROP TABLE IF EXISTS %s", table)
		if drv.Dialect() == dialect.Postgres {
			query += " CASCADE"
		}
		if err := drv.Exec(ctx, query, nil, nil); err != nil {
			return fmt.Errorf("drop legacy table %s: %w", table, err)
		}
	}
	return nil
}

type systemVersionReader struct{ client *ent.Client }

func (r *systemVersionReader) version(ctx context.Context) (string, error) {
	var rows *entsql.Rows
	if err := ent.RawDriver(r.client).Query(ctx, "SELECT value FROM systems WHERE key = 'version' LIMIT 1", nil, &rows); err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", nil
	}
	var version string
	if err := rows.Scan(&version); err != nil {
		return "", err
	}
	return version, nil
}

type rawDialectDriver interface {
	dialect.ExecQuerier
	Dialect() string
}

func existingLegacyTables(ctx context.Context, drv rawDialectDriver) ([]string, error) {
	var result []string
	for _, table := range legacyModelGroupTables {
		query := "SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?"
		args := []any{table}
		if drv.Dialect() == dialect.Postgres || drv.Dialect() == dialect.MySQL {
			query = "SELECT table_name FROM information_schema.tables WHERE table_schema = CURRENT_SCHEMA() AND table_name = ?"
			if drv.Dialect() == dialect.MySQL {
				query = "SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?"
			}
		}
		var rows *entsql.Rows
		if err := drv.Query(ctx, query, args, &rows); err != nil {
			return nil, err
		}
		found := rows.Next()
		rows.Close()
		if found {
			result = append(result, table)
		}
	}
	return result, nil
}

func mustVersion(value string) *semver.Version {
	version, err := semver.NewVersion(value)
	if err != nil {
		panic(err)
	}
	return version
}
