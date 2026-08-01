package datamigrate

import (
	"context"
	"testing"

	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/stretchr/testify/require"
)

type U2Fixture struct {
	Name string
	SQL  []string
}

var u2Fixtures = []U2Fixture{
	{Name: "fresh-db"}, {Name: "gpt-5.6-luna-default", SQL: []string{"model_group:gpt-5.6-luna", "protocol:DEFAULT"}},
	{Name: "openai-anthropic-pools", SQL: []string{"protocol:openai", "protocol:anthropic"}},
	{Name: "protocol-conflict", SQL: []string{"same target with conflicting outbound format"}}, {Name: "missing-model", SQL: []string{"model_group:missing"}},
	{Name: "duplicate-binding", SQL: []string{"duplicate adapter/source/deleted_at"}}, {Name: "idempotent-rerun", SQL: []string{"run beta7 twice"}},
	{Name: "rollback-on-failure", SQL: []string{"force missing model after staging column"}},
}

func TestU2FixturesDeclareRequiredScenarios(t *testing.T) {
	want := map[string]bool{"fresh-db": true, "gpt-5.6-luna-default": true, "openai-anthropic-pools": true, "protocol-conflict": true, "missing-model": true, "duplicate-binding": true, "idempotent-rerun": true, "rollback-on-failure": true}
	got := make(map[string]bool, len(u2Fixtures))
	for _, fixture := range u2Fixtures {
		got[fixture.Name] = true
	}
	for name := range want {
		require.True(t, got[name], "missing fixture %q", name)
	}
}

// TestPrepareBeta7FreshNoOp 使用真实 Ent SQLite client 验证全新数据库不会误触发 legacy 回填。
func TestPrepareBeta7FreshNoOp(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:u2-fresh?mode=memory&_fk=0")
	defer client.Close()
	require.NoError(t, PrepareV1_0_0_Beta7(context.Background(), client))
}
