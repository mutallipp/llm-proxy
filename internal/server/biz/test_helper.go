package biz

import (
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/pkg/xcache"
	"github.com/mutallipp/llm-proxy/llm/httpclient"
)

func NewChannelServiceForTest(client *ent.Client) *ChannelService {
	mockSysSvc := &SystemService{
		AbstractService: &AbstractService{
			db: client,
		},
		Cache: xcache.NewFromConfig[ent.System](xcache.Config{Mode: xcache.ModeMemory}),
	}

	svc := NewChannelService(ChannelServiceParams{
		CacheConfig:   xcache.Config{Mode: xcache.ModeMemory},
		Ent:           client,
		SystemService: mockSysSvc,
		HttpClient:    httpclient.NewHttpClient(),
	})

	svc.SetEnabledChannelsForTest([]*Channel{})

	return svc
}
