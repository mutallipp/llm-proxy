package datamigrate

import "testing"

// U2Fixture 描述 beta7 迁移的数据库夹具与验收场景；仅作为迁移契约，不在本变更中执行。
type U2Fixture struct {
	Name string
	SQL  []string
}

var u2Fixtures = []U2Fixture{
	{Name: "fresh-db"}, {Name: "gpt-5.6-luna-default", SQL: []string{"model_group:gpt-5.6-luna", "protocol:DEFAULT"}},
	{Name: "openai-anthropic-pools", SQL: []string{"protocol:openai", "protocol:anthropic"}},
	{Name: "protocol-conflict", SQL: []string{"same target with conflicting outbound format"}},
	{Name: "missing-model", SQL: []string{"model_group:missing"}}, {Name: "duplicate-binding", SQL: []string{"duplicate adapter/source/deleted_at"}},
	{Name: "idempotent-rerun", SQL: []string{"run beta7 twice"}}, {Name: "rollback-on-failure", SQL: []string{"force missing model after staging column"}},
}

func TestU2FixturesDeclareRequiredScenarios(t *testing.T) {
	want := []string{"fresh-db", "gpt-5.6-luna-default", "openai-anthropic-pools", "protocol-conflict", "missing-model", "duplicate-binding", "idempotent-rerun", "rollback-on-failure"}
	got := make(map[string]bool, len(u2Fixtures))
	for _, fixture := range u2Fixtures {
		got[fixture.Name] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("missing fixture %q", name)
		}
	}
}
