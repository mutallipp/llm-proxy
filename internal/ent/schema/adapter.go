package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/looplj/axonhub/internal/ent/schema/schematype"
)

type Adapter struct {
	ent.Schema
}

func (Adapter) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
		schematype.SoftDeleteMixin{},
	}
}

func (Adapter) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name", "deleted_at").
			StorageKey("adapters_by_name").
			Unique(),
	}
}

func (Adapter) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Immutable(),
		field.String("display_name").Default(""),
		field.String("inbound_api_format"),
		field.Enum("status").Values("enabled", "disabled", "archived").Default("disabled"),
		field.String("remark").Optional().Nillable(),
	}
}

func (Adapter) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("model_bindings", AdapterModelBinding.Type),
	}
}

func (Adapter) Annotations() []schema.Annotation {
	return []schema.Annotation{entgql.Skip(entgql.SkipAll)}
}
