package datamigrate_test

import (
	"context"
	"testing"

	"entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/ent/migrate/datamigrate"
)

func TestDropLegacyModelGroupTablesFreshDatabaseIsNoop(t *testing.T) {
	client := newLegacyModelGroupDropClient(t, "legacy-model-group-fresh")
	ctx := context.Background()

	require.NoError(t, datamigrate.DropLegacyModelGroupTables(ctx, client))
}

func TestDropLegacyModelGroupTablesRequiresBeta7Marker(t *testing.T) {
	client := newLegacyModelGroupDropClient(t, "legacy-model-group-marker")
	ctx := context.Background()
	createLegacyModelGroupTables(t, client)
	setSystemVersion(t, client, "v1.0.0-beta6")

	err := datamigrate.DropLegacyModelGroupTables(ctx, client)
	require.ErrorContains(t, err, "beta7 handoff marker missing")
	assertLegacyModelGroupTablesExist(t, client)
}

func TestDropLegacyModelGroupTablesRejectsOldBindingColumn(t *testing.T) {
	client := newLegacyModelGroupDropClient(t, "legacy-model-group-old-column")
	ctx := context.Background()
	createLegacyModelGroupTables(t, client)
	setSystemVersion(t, client, "v1.0.0-beta7")
	execRaw(t, client, "ALTER TABLE adapter_model_bindings ADD COLUMN model_group_id INTEGER")

	err := datamigrate.DropLegacyModelGroupTables(ctx, client)
	require.ErrorContains(t, err, "adapter_model_bindings.model_group_id still exists")
	assertLegacyModelGroupTablesExist(t, client)
}

func TestDropLegacyModelGroupTablesDropsInDependencyOrder(t *testing.T) {
	client := newLegacyModelGroupDropClient(t, "legacy-model-group-order")
	ctx := context.Background()
	createLegacyModelGroupTables(t, client)
	setSystemVersion(t, client, "v1.0.0-beta7")
	execRaw(t, client, "INSERT INTO model_groups (id) VALUES (1)")
	execRaw(t, client, "INSERT INTO model_group_protocols (id, group_id) VALUES (1, 1)")
	execRaw(t, client, "INSERT INTO model_group_targets (id, protocol_id) VALUES (1, 1)")

	require.NoError(t, datamigrate.DropLegacyModelGroupTables(ctx, client))
	for _, table := range []string{"model_group_targets", "model_group_protocols", "model_groups"} {
		require.False(t, rawTableExists(t, client, table), "table %s should be dropped", table)
	}
}

func TestDropLegacyModelGroupTablesRollsBackAndStopsAfterDropFailure(t *testing.T) {
	client := newLegacyModelGroupDropClient(t, "legacy-model-group-rollback")
	ctx := context.Background()
	createLegacyModelGroupTables(t, client)
	setSystemVersion(t, client, "v1.0.0-beta7")
	execRaw(t, client, "CREATE TABLE protocol_references (protocol_id INTEGER REFERENCES model_group_protocols(id))")
	execRaw(t, client, "INSERT INTO model_groups (id) VALUES (1)")
	execRaw(t, client, "INSERT INTO model_group_protocols (id, group_id) VALUES (1, 1)")
	execRaw(t, client, "INSERT INTO protocol_references (protocol_id) VALUES (1)")

	err := datamigrate.DropLegacyModelGroupTables(ctx, client)
	require.ErrorContains(t, err, "drop legacy table model_group_protocols")
	assertLegacyModelGroupTablesExist(t, client)
}

func newLegacyModelGroupDropClient(t *testing.T, name string) *ent.Client {
	t.Helper()
	return enttest.NewEntClient(t, "sqlite3", "file:"+name+"?mode=memory&_fk=1")
}

func createLegacyModelGroupTables(t *testing.T, client *ent.Client) {
	t.Helper()
	execRaw(t, client, "CREATE TABLE model_groups (id INTEGER PRIMARY KEY)")
	execRaw(t, client, "CREATE TABLE model_group_protocols (id INTEGER PRIMARY KEY, group_id INTEGER REFERENCES model_groups(id))")
	execRaw(t, client, "CREATE TABLE model_group_targets (id INTEGER PRIMARY KEY, protocol_id INTEGER REFERENCES model_group_protocols(id))")
}

func setSystemVersion(t *testing.T, client *ent.Client, version string) {
	t.Helper()
	ctx := authz.WithTestBypass(context.Background())
	client.System.Create().SetKey("system_version").SetValue(version).SaveX(ctx)
}

func execRaw(t *testing.T, client *ent.Client, query string, args ...any) {
	t.Helper()
	require.NoError(t, ent.RawDriver(client).Exec(context.Background(), query, args, nil))
}

func rawTableExists(t *testing.T, client *ent.Client, table string) bool {
	t.Helper()
	rows := &sql.Rows{}
	require.NoError(t, ent.RawDriver(client).Query(
		context.Background(),
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?",
		[]any{table},
		rows,
	))
	defer func() { require.NoError(t, rows.Close()) }()
	if !rows.Next() {
		require.NoError(t, rows.Err())
		return false
	}
	require.NoError(t, rows.Err())
	return true
}

func assertLegacyModelGroupTablesExist(t *testing.T, client *ent.Client) {
	t.Helper()
	for _, table := range []string{"model_group_targets", "model_group_protocols", "model_groups"} {
		require.True(t, rawTableExists(t, client, table), "table %s should remain", table)
	}
}
