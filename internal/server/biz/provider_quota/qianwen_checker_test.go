package provider_quota

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/channel"
	"github.com/mutallipp/llm-proxy/internal/objects"
	"github.com/mutallipp/llm-proxy/llm/httpclient"
)

// qianwenTestChannel 构造一个带控制台 Cookie 的 bailian 渠道
func qianwenTestChannel(cookie string) *ent.Channel {
	return &ent.Channel{
		Type: channel.TypeBailian,
		Settings: &objects.ChannelSettings{
			ProviderQuota: &objects.ChannelProviderQuotaSettings{
				Qianwen: &objects.QianwenQuotaSettings{AuthCookie: cookie},
			},
		},
	}
}

func TestQianwenCheckQuota(t *testing.T) {
	httpClient := httpclient.NewHttpClientWithClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodPost, req.Method)
			require.Equal(t, "https://cs-data.qianwenai.com/data/api.json?action=BroadScopeAspnGateway&product=sfm_bailian&api=zeldaHttp.apikeyMgr.%2Ftokenplan%2Fpersonal%2Fapi%2Fv2%2Fusage", req.URL.String())
			require.Equal(t, "login_qianwenai_ticket=test-ticket", req.Header.Get("Cookie"))
			require.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))

			// 校验表单体包含网关路由参数
			bodyBytes, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			form, err := url.ParseQuery(string(bodyBytes))
			require.NoError(t, err)
			require.Equal(t, "sfm_bailian", form.Get("product"))
			require.Contains(t, form.Get("params"), "/tokenplan/personal/api/v2/usage")

			body := `{
				"data":{
					"DataV2":{
						"ret":["SUCCESS::接口调用成功"],
						"data":{
							"success":true,
							"code":"SUCCESS",
							"data":{
								"per5HourPercentage":0.29,
								"per1WeekPercentage":0.12,
								"per5HourResetTime":1785886860000,
								"per1WeekResetTime":1786277940000
							}
						}
					}
				},
				"successResponse":true
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	})

	checker := NewQianwenQuotaChecker(httpClient)
	quota, err := checker.CheckQuota(context.Background(), qianwenTestChannel(" login_qianwenai_ticket=test-ticket "))
	require.NoError(t, err)

	require.Equal(t, "available", quota.Status)
	require.True(t, quota.Ready)
	require.Equal(t, "qianwen", quota.ProviderType)
	require.Len(t, quota.Limits, 2)
	require.InDelta(t, 0.29, quota.Limits[0].UsageRatio, 0.001)
	require.InDelta(t, 0.12, quota.Limits[1].UsageRatio, 0.001)
	require.NotNil(t, quota.NextResetAt)

	rows, ok := quota.RawData["rows"].([]minimaxModelRow)
	require.True(t, ok)
	require.Len(t, rows, 1)
	require.InDelta(t, 29.0, rows[0].IntervalPercent, 0.001)
	require.InDelta(t, 12.0, rows[0].WeeklyPercent, 0.001)
	require.NotNil(t, rows[0].IntervalResetAt)
	require.NotNil(t, rows[0].WeeklyResetAt)
}

func TestQianwenCheckQuotaWarning(t *testing.T) {
	httpClient := httpclient.NewHttpClientWithClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := `{
				"data":{
					"DataV2":{
						"ret":["SUCCESS::接口调用成功"],
						"data":{
							"success":true,
							"code":"SUCCESS",
							"data":{
								"per5HourPercentage":0.85,
								"per1WeekPercentage":0.4,
								"per5HourResetTime":1785886860000,
								"per1WeekResetTime":1786277940000
							}
						}
					}
				}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	})

	checker := NewQianwenQuotaChecker(httpClient)
	quota, err := checker.CheckQuota(context.Background(), qianwenTestChannel("t"))
	require.NoError(t, err)
	require.Equal(t, "warning", quota.Status)
	require.True(t, quota.Ready)
}

func TestQianwenCheckQuotaNoCredentials(t *testing.T) {
	checker := NewQianwenQuotaChecker(httpclient.NewHttpClientWithClient(&http.Client{}))
	_, err := checker.CheckQuota(context.Background(), qianwenTestChannel("  "))
	require.Error(t, err)
	require.Contains(t, err.Error(), "channel has no credentials")
}

func TestQianwenSupportsChannel(t *testing.T) {
	checker := NewQianwenQuotaChecker(nil)
	require.True(t, checker.SupportsChannel(&ent.Channel{Type: channel.TypeBailian}))
	require.True(t, checker.SupportsChannel(&ent.Channel{Type: channel.TypeBailianAnthropic}))
	require.False(t, checker.SupportsChannel(&ent.Channel{Type: channel.TypeMinimax}))
}

func TestQianwenAPIError(t *testing.T) {
	httpClient := httpclient.NewHttpClientWithClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := `{"data":{"DataV2":{"ret":["FAIL::登录已过期"],"data":{"success":false,"code":"FAIL"}}}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	})

	checker := NewQianwenQuotaChecker(httpClient)
	_, err := checker.CheckQuota(context.Background(), qianwenTestChannel("t"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "登录已过期")
}
