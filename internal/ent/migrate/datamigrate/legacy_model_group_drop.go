package datamigrate

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/Masterminds/semver/v3"

	"github.com/mutallipp/llm-proxy/internal/ent"
)

var legacyModelGroupTables = []string{"model_group_targets", "model_group_protocols", "model_groups"}

// DropLegacyModelGroupTables 仅在 beta7 handoff 已由系统版本标记确认后删除旧表。
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
	if err := ensureModelGroupBindingHandoff(ctx, drv); err != nil {
		return err
	}

	// MySQL 的 DDL 可能隐式提交，不能冒险执行部分删除。
	if drv.Dialect() == dialect.MySQL {
		return fmt.Errorf("refusing to drop legacy model-group tables: MySQL DDL transaction is unsupported")
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin legacy model-group table drop transaction: %w", err)
	}
	txClient := tx.Client()
	for _, table := range existing {
		query := fmt.Sprintf("DROP TABLE IF EXISTS %s", table)
		if drv.Dialect() == dialect.Postgres {
			query += " CASCADE"
		}
		if err := ent.RawDriver(txClient).Exec(ctx, query, []any{}, nil); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("drop legacy table %s: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("commit legacy model-group table drop transaction: %w", err)
	}
	return nil
}

func ensureModelGroupBindingHandoff(ctx context.Context, drv rawDialectDriver) (err error) {
	exists, err := tableExists(ctx, drv, "adapter_model_bindings")
	if err != nil || !exists {
		return err
	}
	if drv.Dialect() == dialect.SQLite {
		rows := &entsql.Rows{}
		if err = drv.Query(ctx, "PRAGMA table_info(adapter_model_bindings)", []any{}, rows); err != nil {
			return err
		}
		defer func() { err = errors.Join(err, rows.Close()) }()
		for rows.Next() {
			var cid int
			var name, typ string
			var notnull, pk int
			var dflt any
			if err = rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				return err
			}
			if name == "model_group_id" {
				return fmt.Errorf("refusing to drop legacy model-group tables: handoff incomplete, adapter_model_bindings.model_group_id still exists")
			}
		}
		return rows.Err()
	}
	query := "SELECT column_name FROM information_schema.columns WHERE table_schema = CURRENT_SCHEMA() AND table_name = $1 AND column_name = $2"
	if drv.Dialect() == dialect.MySQL {
		query = "SELECT column_name FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?"
	}
	rows := &entsql.Rows{}
	if err = drv.Query(ctx, query, []any{"adapter_model_bindings", "model_group_id"}, rows); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	if rows.Next() {
		return fmt.Errorf("refusing to drop legacy model-group tables: handoff incomplete, adapter_model_bindings.model_group_id still exists")
	}
	return rows.Err()
}

func tableExists(ctx context.Context, drv rawDialectDriver, table string) (exists bool, err error) {
	query, args := "SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", []any{table}
	if drv.Dialect() == dialect.Postgres {
		query = "SELECT table_name FROM information_schema.tables WHERE table_schema = CURRENT_SCHEMA() AND table_name = $1"
	}
	if drv.Dialect() == dialect.MySQL {
		query = "SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?"
	}
	rows := &entsql.Rows{}
	if err = drv.Query(ctx, query, args, rows); err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	exists = rows.Next()
	return exists, rows.Err()
}

type systemVersionReader struct{ client *ent.Client }

func (r *systemVersionReader) version(ctx context.Context) (version string, err error) {
	rows := &entsql.Rows{}
	if err = ent.RawDriver(r.client).Query(ctx, "SELECT value FROM systems WHERE key = 'system_version' LIMIT 1", []any{}, rows); err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	if !rows.Next() {
		return "", rows.Err()
	}
	if err = rows.Scan(&version); err != nil {
		return "", err
	}
	return version, rows.Err()
}

type rawDialectDriver interface {
	dialect.ExecQuerier
	Dialect() string
}

func existingLegacyTables(ctx context.Context, drv rawDialectDriver) ([]string, error) {
	var result []string
	for _, table := range legacyModelGroupTables {
		found, err := tableExists(ctx, drv, table)
		if err != nil {
			return nil, err
		}
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
