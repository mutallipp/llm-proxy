package datamigrate

import (
	"context"
	"fmt"
	"slices"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/model"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

// V1_0_0_Beta7 migrates legacy ModelGroup routing into Model protocol pools.
type V1_0_0_Beta7 struct{}

func NewV1_0_0_Beta7() DataMigrator   { return &V1_0_0_Beta7{} }
func (*V1_0_0_Beta7) Version() string { return "v1.0.0-beta7" }

func (v *V1_0_0_Beta7) Migrate(ctx context.Context, client *ent.Client) (err error) {
	ctx = authz.WithSystemBypass(ctx, "database-migrate")
	ctx, tx, err := client.OpenTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	db := ent.FromContext(ctx)
	groups, err := db.ModelGroup.Query().WithProtocols(func(q *ent.ModelGroupProtocolQuery) {
		q.WithTargets()
	}).Order(ent.Asc("name")).All(ctx)
	if err != nil {
		return err
	}
	for _, group := range groups {
		model, err := db.Model.Query().Where(modelID(group.Name)).Only(ctx)
		if err != nil {
			return fmt.Errorf("model-group %q: missing model: %w", group.Name, err)
		}
		settings := model.Settings
		if settings == nil {
			settings = &objects.ModelSettings{}
		}
		if settings.ProtocolPools == nil {
			settings.ProtocolPools = map[string][]*objects.ModelAssociation{}
		}
		pools := make(map[string][]*objects.ModelAssociation, len(group.Edges.Protocols))
		for _, protocol := range group.Edges.Protocols {
			items := make([]*objects.ModelAssociation, 0, len(protocol.Edges.Targets))
			for _, target := range protocol.Edges.Targets {
				if target.OutboundAPIFormat != protocol.InboundAPIFormat {
					return fmt.Errorf("model-group %q protocol %q target %d: outbound protocol %q conflicts", group.Name, protocol.InboundAPIFormat, target.ID, target.OutboundAPIFormat)
				}
				items = append(items, &objects.ModelAssociation{Type: "channel_model", Priority: target.Priority, Disabled: !target.Enabled, ChannelModel: &objects.ChannelModelAssociation{ChannelID: target.ChannelID, ModelID: target.TargetModelID}})
			}
			slices.SortFunc(items, func(a, b *objects.ModelAssociation) int {
				if a.Priority != b.Priority {
					return a.Priority - b.Priority
				}
				if a.ChannelModel.ChannelID != b.ChannelModel.ChannelID {
					return a.ChannelModel.ChannelID - b.ChannelModel.ChannelID
				}
				return compare(a.ChannelModel.ModelID, b.ChannelModel.ModelID)
			})
			pools[protocol.InboundAPIFormat] = items
		}
		for key, pool := range pools {
			if old, ok := settings.ProtocolPools[key]; ok && !equalAssociations(old, pool) {
				return fmt.Errorf("model %q protocol pool %q already exists with different content", group.Name, key)
			}
			settings.ProtocolPools[key] = pool
		}
		if err = settings.ValidateProtocolPools(); err != nil {
			return fmt.Errorf("model %q: %w", group.Name, err)
		}
		if _, err = db.Model.UpdateOne(model).SetSettings(settings).Save(ctx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func modelID(id string) func(*ent.ModelQuery) {
	return func(q *ent.ModelQuery) { q.Where(model.ModelIDEQ(id)) }
}
func compare(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
func equalAssociations(a, b []*objects.ModelAssociation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Priority != b[i].Priority || a[i].Disabled != b[i].Disabled || a[i].ChannelModel.ChannelID != b[i].ChannelModel.ChannelID || a[i].ChannelModel.ModelID != b[i].ChannelModel.ModelID {
			return false
		}
	}
	return true
}
