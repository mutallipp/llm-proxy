package gql

import (
	"github.com/samber/lo"

	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

func filterChannelsByProjectProfile(channels []*ent.Channel, projectProfile *objects.ProjectProfile) []*ent.Channel {
	if projectProfile == nil {
		return channels
	}

	filtered := channels
	if len(projectProfile.ChannelIDs) > 0 {
		filtered = lo.Filter(filtered, func(ch *ent.Channel, _ int) bool {
			return lo.Contains(projectProfile.ChannelIDs, ch.ID)
		})
	}

	if len(projectProfile.ChannelTags) > 0 {
		filtered = lo.Filter(filtered, func(ch *ent.Channel, _ int) bool {
			return projectProfile.MatchChannelTags(ch.Tags)
		})
	}

	return filtered
}
