package datamigrate

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/mutallipp/llm-proxy/internal/ent"
)

// PrepareV1_0_0_Beta7 在 Ent schema cutover 前回填 adapter binding 的 legacy 外键。
func PrepareV1_0_0_Beta7(ctx context.Context, client *ent.Client) error {
	drv := ent.RawDriver(client)
	exists, err := tableExists(ctx, drv)
	if err != nil || !exists {
		return err
	}
	legacy, err := columnExists(ctx, drv, "model_group_id")
	if err != nil || !legacy {
		return err
	}

	if drv.Dialect() == "mysql" {
		return fmt.Errorf("mysql legacy adapter migration is unsupported: DDL may implicitly commit")
	}
	tx, err := drv.Tx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := tx.Exec(ctx, "ALTER TABLE adapter_model_bindings ADD COLUMN model_id INTEGER", nil, nil); err != nil {
		var check *entsql.Rows
		if qerr := tx.Query(ctx, "SELECT model_id FROM adapter_model_bindings LIMIT 1", nil, &check); qerr != nil {
			return fmt.Errorf("prepare model_id: %w", err)
		}
		check.Close()
	}
	var rows *entsql.Rows
	if err := tx.Query(ctx, "SELECT id, model_group_id FROM adapter_model_bindings WHERE model_group_id IS NOT NULL", nil, &rows); err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, groupID int
		if err := rows.Scan(&id, &groupID); err != nil {
			return err
		}
		var groups *entsql.Rows
		if err := tx.Query(ctx, placeholder(tx.Dialect(), "SELECT name FROM model_groups WHERE id = %s", 1), []any{groupID}, &groups); err != nil {
			return err
		}
		if !groups.Next() {
			groups.Close()
			return fmt.Errorf("model-group id %d: not found", groupID)
		}
		var name string
		if err := groups.Scan(&name); err != nil {
			groups.Close()
			return err
		}
		groups.Close()
		var models *entsql.Rows
		if err := tx.Query(ctx, placeholder(tx.Dialect(), "SELECT id FROM models WHERE model_id = %s", 1), []any{name}, &models); err != nil {
			return err
		}
		if !models.Next() {
			models.Close()
			return fmt.Errorf("model-group %q: missing model", name)
		}
		var modelID int
		if err := models.Scan(&modelID); err != nil {
			models.Close()
			return err
		}
		models.Close()
		if err := tx.Exec(ctx, placeholder(tx.Dialect(), "UPDATE adapter_model_bindings SET model_id = %s WHERE id = %s", 2), []any{modelID, id}, nil); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func columnExists(ctx context.Context, drv interface {
	dialect.ExecQuerier
	Dialect() string
}, column string) (bool, error) {
	q, args := "SELECT name FROM pragma_table_info('adapter_model_bindings') WHERE name = ?", []any{column}
	if drv.Dialect() == "postgres" {
		q, args = "SELECT column_name FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'adapter_model_bindings' AND column_name = $1", []any{column}
	}
	if drv.Dialect() == "mysql" {
		q, args = "SELECT column_name FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'adapter_model_bindings' AND column_name = ?", []any{column}
	}
	var rows *entsql.Rows
	if err := drv.Query(ctx, q, args, &rows); err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), nil
}

func tableExists(ctx context.Context, drv interface {
	dialect.ExecQuerier
	Dialect() string
}) (bool, error) {
	q := "SELECT name FROM sqlite_master WHERE type='table' AND name='adapter_model_bindings'"
	if drv.Dialect() == "postgres" {
		q = "SELECT table_name FROM information_schema.tables WHERE table_name='adapter_model_bindings'"
	}
	if drv.Dialect() == "mysql" {
		q = "SELECT table_name FROM information_schema.tables WHERE table_name='adapter_model_bindings'"
	}
	var rows *entsql.Rows
	if err := drv.Query(ctx, q, nil, &rows); err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), nil
}

func placeholder(dialectName, format string, count int) string {
	for i := 1; i <= count; i++ {
		repl := "?"
		if dialectName == "postgres" {
			repl = fmt.Sprintf("$%d", i)
		}
		format = replaceFirst(format, "%s", repl)
	}
	return format
}

func replaceFirst(value, old, repl string) string {
	for i := 0; i+len(old) <= len(value); i++ {
		if value[i:i+len(old)] == old {
			return value[:i] + repl + value[i+len(old):]
		}
	}
	return value
}
