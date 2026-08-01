package objects

import (
	"encoding/json"
	"testing"
)

func TestModelSettingsProtocolPoolsJSON(t *testing.T) {
	settings := &ModelSettings{ProtocolPools: map[string][]*ModelAssociation{
		"openai":    {{Type: "channel_model", Priority: 2, ChannelModel: &ChannelModelAssociation{ChannelID: 1, ModelID: "gpt"}}},
		"anthropic": {{Type: "channel_model", Priority: 1, ChannelModel: &ChannelModelAssociation{ChannelID: 2, ModelID: "claude"}}},
	}}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ModelSettings
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.ProtocolPools["openai"]) != 1 || len(decoded.ProtocolPools["anthropic"]) != 1 {
		t.Fatal("协议池未按 key 保留")
	}
	if err := decoded.ValidateProtocolPools(); err != nil {
		t.Fatal(err)
	}
}

func TestModelSettingsProtocolPoolsRejectsInvalidAndDuplicate(t *testing.T) {
	invalid := &ModelSettings{ProtocolPools: map[string][]*ModelAssociation{"gemini": {}}}
	if err := invalid.ValidateProtocolPools(); err == nil {
		t.Fatal("非法协议应报错")
	}
	duplicate := &ModelSettings{ProtocolPools: map[string][]*ModelAssociation{"openai": {
		{Type: "channel_model", ChannelModel: &ChannelModelAssociation{ChannelID: 1, ModelID: "gpt"}},
		{Type: "channel_model", ChannelModel: &ChannelModelAssociation{ChannelID: 1, ModelID: "gpt"}},
	}}}
	if err := duplicate.ValidateProtocolPools(); err == nil {
		t.Fatal("重复目标应报错")
	}
}
