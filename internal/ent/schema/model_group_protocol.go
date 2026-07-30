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

type ModelGroupProtocol struct {
	ent.Schema
}

func (ModelGroupProtocol) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
		schematype.SoftDeleteMixin{},
	}
}

func (ModelGroupProtocol) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("model_group_id", "inbound_api_format", "deleted_at").
			StorageKey("model_group_protocols_by_group_format").
			Unique(),
	}
}

func (ModelGroupProtocol) Fields() []ent.Field {
	return []ent.Field{
		field.Int("model_group_id").Immutable(),
		field.String("inbound_api_format"),
		field.Bool("enabled").Default(true),
		field.String("remark").Optional().Nillable(),
	}
}

func (ModelGroupProtocol) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("model_group", ModelGroup.Type).
			Ref("protocols").
			Field("model_group_id").
			Immutable().
			Required().
			Unique(),
		edge.To("targets", ModelGroupTarget.Type),
	}
}

func (ModelGroupProtocol) Annotations() []schema.Annotation {
	return []schema.Annotation{entgql.Skip(entgql.SkipAll)}
}
