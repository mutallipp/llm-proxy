package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/looplj/axonhub/internal/ent/schema/schematype"
	"github.com/looplj/axonhub/internal/objects"
)

type ModelGroupTarget struct {
	ent.Schema
}

func (ModelGroupTarget) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
		schematype.SoftDeleteMixin{},
	}
}

func (ModelGroupTarget) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("model_group_protocol_id", "channel_id", "target_model_id", "outbound_api_format", "deleted_at").
			StorageKey("model_group_targets_by_protocol_channel_model").
			Unique(),
		index.Fields("channel_id", "deleted_at").
			StorageKey("model_group_targets_by_channel"),
	}
}

func (ModelGroupTarget) Fields() []ent.Field {
	return []ent.Field{
		field.Int("model_group_protocol_id").Immutable(),
		field.Int("channel_id").Immutable(),
		field.String("target_model_id"),
		field.String("outbound_api_format"),
		field.Int("priority").Default(0),
		field.Bool("enabled").Default(true),
		field.JSON("capabilities", objects.AdapterTargetCapabilities{}).
			Default(objects.AdapterTargetCapabilities{
				InputModalities:  []string{},
				OutputModalities: []string{},
			}),
		field.String("remark").Optional().Nillable(),
	}
}

func (ModelGroupTarget) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("model_group_protocol", ModelGroupProtocol.Type).
			Ref("targets").
			Field("model_group_protocol_id").
			Immutable().
			Required().
			Unique(),
		edge.From("channel", Channel.Type).
			Ref("model_group_targets").
			Field("channel_id").
			Immutable().
			Required().
			Unique(),
	}
}

func (ModelGroupTarget) Annotations() []schema.Annotation {
	return []schema.Annotation{entgql.Skip(entgql.SkipAll)}
}
