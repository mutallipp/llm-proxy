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

type AdapterModelBinding struct {
	ent.Schema
}

func (AdapterModelBinding) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
		schematype.SoftDeleteMixin{},
	}
}

func (AdapterModelBinding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("adapter_id", "source_model_id", "deleted_at").
			StorageKey("adapter_model_bindings_by_adapter_model").
			Unique(),
		index.Fields("model_group_id", "deleted_at").
			StorageKey("adapter_model_bindings_by_model_group"),
	}
}

func (AdapterModelBinding) Fields() []ent.Field {
	return []ent.Field{
		field.Int("adapter_id").Immutable(),
		field.String("source_model_id"),
		field.Int("model_group_id").Immutable(),
		field.Bool("enabled").Default(true),
		field.String("remark").Optional().Nillable(),
	}
}

func (AdapterModelBinding) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("adapter", Adapter.Type).
			Ref("model_bindings").
			Field("adapter_id").
			Immutable().
			Required().
			Unique(),
		edge.From("model_group", ModelGroup.Type).
			Ref("adapter_bindings").
			Field("model_group_id").
			Immutable().
			Required().
			Unique(),
	}
}

func (AdapterModelBinding) Annotations() []schema.Annotation {
	return []schema.Annotation{entgql.Skip(entgql.SkipAll)}
}
