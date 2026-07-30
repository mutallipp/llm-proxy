package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/mutallipp/llm-proxy/internal/ent/schema/schematype"
)

type ModelGroup struct {
	ent.Schema
}

func (ModelGroup) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
		schematype.SoftDeleteMixin{},
	}
}

func (ModelGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name", "deleted_at").
			StorageKey("model_groups_by_name").
			Unique(),
	}
}

func (ModelGroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.String("display_name").Default(""),
		field.Enum("status").Values("enabled", "disabled", "archived").Default("disabled"),
		field.Enum("selection_strategy").Values("priority_failover").Default("priority_failover"),
		field.String("remark").Optional().Nillable(),
	}
}

func (ModelGroup) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("adapter_bindings", AdapterModelBinding.Type),
		edge.To("protocols", ModelGroupProtocol.Type),
	}
}

func (ModelGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{entgql.Skip(entgql.SkipAll)}
}
