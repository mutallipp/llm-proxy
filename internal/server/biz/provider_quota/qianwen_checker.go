package provider_quota

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/channel"
	"github.com/mutallipp/llm-proxy/llm/httpclient"
)

// qianwenQuotaGatewayURL 是千问控制台加载 Token Plan 用量时调用的网关地址。
// 千问没有可用 API Key 调用的配额查询接口，因此复用控制台会话 Cookie 轮询。
const qianwenQuotaGatewayURL = "https://cs-data.qianwenai.com/data/api.json?action=BroadScopeAspnGateway&product=sfm_bailian&api=zeldaHttp.apikeyMgr.%2Ftokenplan%2Fpersonal%2Fapi%2Fv2%2Fusage"

// qianwenUsageAPI 是网关 params.Api 字段内部的用量接口路径。
const qianwenUsageAPI = "zeldaHttp.apikeyMgr./tokenplan/personal/api/v2/usage"

type QianwenQuotaChecker struct {
	httpClient *httpclient.HttpClient
}

func NewQianwenQuotaChecker(httpClient *httpclient.HttpClient) *QianwenQuotaChecker {
	return &QianwenQuotaChecker{httpClient: httpClient}
}

func (c *QianwenQuotaChecker) SupportsChannel(ch *ent.Channel) bool {
	return ch.Type == channel.TypeBailian || ch.Type == channel.TypeBailianAnthropic
}

func (c *QianwenQuotaChecker) CheckQuota(ctx context.Context, ch *ent.Channel) (QuotaData, error) {
	// 千问配额轮询依赖控制台会话 Cookie，而非渠道 API Key
	authCookie := ""
	if ch.Settings != nil && ch.Settings.ProviderQuota != nil && ch.Settings.ProviderQuota.Qianwen != nil {
		authCookie = strings.TrimSpace(ch.Settings.ProviderQuota.Qianwen.AuthCookie)
	}
	if authCookie == "" {
		return QuotaData{}, fmt.Errorf("channel has no credentials")
	}

	params, err := json.Marshal(map[string]any{
		"Api": qianwenUsageAPI,
		"Data": map[string]any{
			"cornerstoneParam": map[string]any{
				"domain":      "platform.qianwenai.com",
				"consoleSite": "QIANWENAI",
				"console":     "ONE_CONSOLE",
				"xsp_lang":    "zh-CN",
				"protocol":    "V2",
				"productCode": "p_efm",
			},
		},
		"V": "1.0",
	})
	if err != nil {
		return QuotaData{}, fmt.Errorf("failed to build qianwen quota params: %w", err)
	}

	form := url.Values{}
	form.Set("product", "sfm_bailian")
	form.Set("action", "BroadScopeAspnGateway")
	form.Set("region", "cn-beijing")
	form.Set("params", string(params))

	request := httpclient.NewRequestBuilder().
		WithMethod(http.MethodPost).
		WithURL(qianwenQuotaGatewayURL).
		WithHeader("Content-Type", "application/x-www-form-urlencoded").
		WithHeader("Cookie", authCookie).
		WithHeader("Origin", "https://platform.qianwenai.com").
		WithHeader("Referer", "https://platform.qianwenai.com/").
		WithBody(form.Encode()).
		Build()

	hc := c.httpClient
	if ch.Settings != nil && ch.Settings.Proxy != nil {
		hc = c.httpClient.WithProxy(ch.Settings.Proxy)
	}

	resp, err := hc.Do(ctx, request)
	if err != nil {
		return QuotaData{}, fmt.Errorf("qianwen quota request failed: %w", err)
	}

	return parseQianwenResponse(resp.Body)
}

// qianwenUsageResponse 匹配网关返回结构：
// {data:{DataV2:{ret:[...],data:{success,data:{per5HourPercentage,...}}}}}
type qianwenUsageResponse struct {
	Data struct {
		DataV2 struct {
			Ret  []string `json:"ret"`
			Data struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
				Data    struct {
					Per5HourPercentage float64 `json:"per5HourPercentage"`
					Per1WeekPercentage float64 `json:"per1WeekPercentage"`
					Per5HourResetTime  int64   `json:"per5HourResetTime"`
					Per1WeekResetTime  int64   `json:"per1WeekResetTime"`
				} `json:"data"`
			} `json:"data"`
		} `json:"DataV2"`
	} `json:"data"`
}

// qianwenStatusForRatio 把已用比例映射为统一状态。
func qianwenStatusForRatio(ratio float64) string {
	switch {
	case ratio >= 1:
		return "exhausted"
	case ratio >= WarningThresholdRatio:
		return "warning"
	default:
		return "available"
	}
}

func parseQianwenResponse(body []byte) (QuotaData, error) {
	var response qianwenUsageResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return QuotaData{}, fmt.Errorf("failed to parse qianwen quota response: %w", err)
	}

	dataV2 := response.Data.DataV2
	if len(dataV2.Ret) > 0 && !strings.HasPrefix(dataV2.Ret[0], "SUCCESS") {
		return QuotaData{}, fmt.Errorf("qianwen quota API error: %s", strings.Join(dataV2.Ret, "; "))
	}
	if !dataV2.Data.Success && dataV2.Data.Code != "SUCCESS" {
		return QuotaData{}, fmt.Errorf("qianwen quota API returned unsuccessful (code %s)", dataV2.Data.Code)
	}

	usage := dataV2.Data.Data

	var reset5h, resetWeek *time.Time
	if usage.Per5HourResetTime > 0 {
		t := time.UnixMilli(usage.Per5HourResetTime)
		reset5h = &t
	}
	if usage.Per1WeekResetTime > 0 {
		t := time.UnixMilli(usage.Per1WeekResetTime)
		resetWeek = &t
	}

	status5h := qianwenStatusForRatio(usage.Per5HourPercentage)
	statusWeek := qianwenStatusForRatio(usage.Per1WeekPercentage)

	overallStatus := worseStatus(status5h, statusWeek)

	var nextResetAt *time.Time
	for _, t := range []*time.Time{reset5h, resetWeek} {
		if t == nil {
			continue
		}
		if nextResetAt == nil || t.Before(*nextResetAt) {
			nextResetAt = t
		}
	}

	limits := []QuotaLimitStatus{
		NewTokenLimitStatus(status5h, usage.Per5HourPercentage, reset5h),
		NewTokenLimitStatus(statusWeek, usage.Per1WeekPercentage, resetWeek),
	}

	// 复用 minimax 的行结构，前端用同一套进度条渲染
	row := minimaxModelRow{
		ModelName:            "Token Plan",
		IntervalUsedPercent:  usage.Per5HourPercentage * 100,
		IntervalTotalPercent: 100,
		IntervalPercent:      usage.Per5HourPercentage * 100,
		IntervalStatus:       status5h,
		WeeklyUsedPercent:    usage.Per1WeekPercentage * 100,
		WeeklyTotalPercent:   100,
		WeeklyPercent:        usage.Per1WeekPercentage * 100,
		WeeklyStatus:         statusWeek,
	}
	if reset5h != nil {
		s := reset5h.Format(time.RFC3339)
		row.IntervalResetAt = &s
	}
	if resetWeek != nil {
		s := resetWeek.Format(time.RFC3339)
		row.WeeklyResetAt = &s
	}

	return QuotaData{
		Status:       overallStatus,
		ProviderType: "qianwen",
		Ready:        IsReadyStatus(overallStatus),
		NextResetAt:  nextResetAt,
		Limits:       limits,
		RawData: map[string]any{
			"rows": []minimaxModelRow{row},
		},
	}, nil
}
