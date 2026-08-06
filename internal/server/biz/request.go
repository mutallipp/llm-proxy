//nolint:nilerr // Checked.
package biz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/eko/gocache/lib/v4/store"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/contexts"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/request"
	"github.com/mutallipp/llm-proxy/internal/ent/requestexecution"
	"github.com/mutallipp/llm-proxy/internal/log"
	"github.com/mutallipp/llm-proxy/internal/objects"
	"github.com/mutallipp/llm-proxy/internal/pkg/xcache"
	"github.com/mutallipp/llm-proxy/internal/pkg/xjson"
	"github.com/mutallipp/llm-proxy/llm"
	"github.com/mutallipp/llm-proxy/llm/httpclient"
)

// RequestService handles request and request execution operations.
type RequestService struct {
	*AbstractService

	SystemService        *SystemService
	UsageLogService      *UsageLogService
	DataStorageService   *DataStorageService
	LiveStreamRegistry   *LiveStreamRegistry
	previousChannelCache xcache.Cache[int]
}

// NewRequestService creates a new RequestService.
func NewRequestService(
	ent *ent.Client,
	cacheConfig xcache.Config,
	systemService *SystemService,
	usageLogService *UsageLogService,
	dataStorageService *DataStorageService,
	liveStreamRegistry *LiveStreamRegistry,
) *RequestService {
	return &RequestService{
		AbstractService: &AbstractService{
			db: ent,
		},
		SystemService:        systemService,
		UsageLogService:      usageLogService,
		DataStorageService:   dataStorageService,
		LiveStreamRegistry:   liveStreamRegistry,
		previousChannelCache: xcache.NewFromConfig[int](cacheConfig),
	}
}

// shouldUseExternalStorage checks if data should be saved to external storage.
// Returns true if the data storage is not primary (database).
func (s *RequestService) shouldUseExternalStorage(_ context.Context, ds *ent.DataStorage) bool {
	if ds == nil {
		return false
	}

	return !ds.Primary
}

// _InvalidRequestBodyJSON returns a JSON object indicating invalid text.
var _InvalidRequestBodyJSON = objects.JSONRawMessage(`{"message":"invalid text"}`)

// requestIDToRelayID 将数值 Request ID 转换为 Relay GID，保持测试结果
// 与 GraphQL Request 节点的定位格式一致。
func requestIDToRelayID(id int) string {
	if id <= 0 {
		return ""
	}

	return fmt.Sprintf("gid://axonhub/Request/%d", id)
}

// GenerateRequestBodyKey generates the storage key for request body.
func GenerateRequestBodyKey(projectID, requestID int) string {
	return fmt.Sprintf("/%d/requests/%d/request_body.json", projectID, requestID)
}

// GenerateResponseBodyKey generates the storage key for response body.
func GenerateResponseBodyKey(projectID, requestID int) string {
	return fmt.Sprintf("/%d/requests/%d/response_body.json", projectID, requestID)
}

// GenerateAudioKey generates the storage key for a generated audio file (TTS).
func GenerateAudioKey(projectID, requestID int, filename string) string {
	name := strings.TrimSpace(filename)
	if name == "" {
		name = "audio.mp3"
	}

	name = filepath.Base(name)

	return fmt.Sprintf("/%d/requests/%d/audio/%s", projectID, requestID, name)
}

// GenerateResponseChunksKey generates the storage key for response chunks.
func GenerateResponseChunksKey(projectID, requestID int) string {
	return fmt.Sprintf("/%d/requests/%d/response_chunks.json", projectID, requestID)
}

// GenerateRequestDirKey generates the storage key for request.
func GenerateRequestDirKey(projectID, requestID int) string {
	return fmt.Sprintf("/%d/requests/%d", projectID, requestID)
}

func GenerateRequestExecutionsDirKey(projectID, requestID int) string {
	return fmt.Sprintf("/%d/requests/%d/executions", projectID, requestID)
}

// GenerateExecutionRequestBodyKey generates the storage key for execution request body.
func GenerateExecutionRequestBodyKey(projectID, requestID, executionID int) string {
	return fmt.Sprintf("/%d/requests/%d/executions/%d/request_body.json", projectID, requestID, executionID)
}

// GenerateExecutionResponseBodyKey generates the storage key for execution response body.
func GenerateExecutionResponseBodyKey(projectID, requestID, executionID int) string {
	return fmt.Sprintf("/%d/requests/%d/executions/%d/response_body.json", projectID, requestID, executionID)
}

// GenerateExecutionResponseChunksKey generates the storage key for execution response chunks.
func GenerateExecutionResponseChunksKey(projectID, requestID, executionID int) string {
	return fmt.Sprintf("/%d/requests/%d/executions/%d/response_chunks.json", projectID, requestID, executionID)
}

// GenerateExecutionRequestDirKey generates the storage key for execution request.
func GenerateExecutionRequestDirKey(projectID, requestID, executionID int) string {
	return fmt.Sprintf("/%d/requests/%d/executions/%d", projectID, requestID, executionID)
}

// CreateRequest creates a new request record.
func (s *RequestService) CreateRequest(
	ctx context.Context,
	llmRequest *llm.Request,
	httpRequest *httpclient.Request,
	format llm.APIFormat,
) (*ent.Request, error) {
	// Get project ID from context.
	// If project ID is not found, use zero.
	// It will be not prsent in the admin pages,
	// e.g: test channel.
	projectID, _ := contexts.GetProjectID(ctx)

	// Decide whether to store the original request body
	storeRequestBody := true
	if policy, err := s.SystemService.StoragePolicy(ctx); err == nil {
		storeRequestBody = policy.StoreRequestBody
	} else {
		log.Warn(ctx, "Failed to get storage policy, defaulting to store request body", log.Cause(err))
	}

	var (
		requestBodyBytes    objects.JSONRawMessage = []byte("{}")
		requestHeadersBytes objects.JSONRawMessage = []byte("{}")
	)

	if storeRequestBody {
		if len(httpRequest.JSONBody) > 0 {
			requestBodyBytes = httpRequest.JSONBody
		} else {
			b, err := xjson.Marshal(httpRequest.Body)
			if err != nil {
				log.Error(ctx, "Failed to serialize request body", log.Cause(err))
				return nil, err
			}

			requestBodyBytes = b
		}

		if httpRequest != nil && len(httpRequest.Headers) > 0 {
			requestHeadersBytes, _ = xjson.Marshal(httpclient.MaskSensitiveHeaders(httpRequest.Headers))
		}
	} // else keep nil -> stored as JSON null

	isStream := false
	if llmRequest.Stream != nil {
		isStream = *llmRequest.Stream
	}

	// Get default data storage
	dataStorage, err := s.DataStorageService.GetDefaultDataStorage(ctx)
	if err != nil {
		log.Warn(ctx, "Failed to get default data storage, request will be created without data storage", log.Cause(err))
	}

	client := s.entFromContext(ctx)
	responseChunksAvailability := request.ResponseChunksAvailabilityNotApplicable
	if isStream {
		responseChunksAvailability = request.ResponseChunksAvailabilityUnavailable
	}
	mut := client.Request.Create().
		SetProjectID(projectID).
		SetModelID(llmRequest.Model).
		SetFormat(string(format)).
		SetSource(contexts.GetSourceOrDefault(ctx, request.SourceAPI)).
		SetStatus(request.StatusProcessing).
		SetStream(isStream).
		SetRequestHeaders(requestHeadersBytes).
		SetRequestBodyAvailability(request.RequestBodyAvailabilityUnavailable).
		SetResponseBodyAvailability(request.ResponseBodyAvailabilityUnavailable).
		SetResponseChunksAvailability(responseChunksAvailability)

	// 测试归属只对 source=test 有意义，普通请求不能写入测试历史。
	if testOrigin := contexts.GetTestOrigin(ctx); testOrigin.Valid() {
		sourceValue := contexts.GetSourceOrDefault(ctx, request.SourceAPI)
		if sourceValue == request.SourceTest {
			mut = mut.
				SetTestOriginType(request.TestOriginType(string(testOrigin.Kind))).
				SetTestOriginID(testOrigin.ID).
				SetTestOriginLabel(testOrigin.Label)
		}
	}

	if httpRequest != nil {
		mut = mut.SetClientIP(httpRequest.ClientIP)
	}

	if llmRequest.ReasoningEffort != "" {
		mut = mut.SetReasoningEffort(llmRequest.ReasoningEffort)
	}

	// Determine if we should store in database or external storage
	useExternalStorage := storeRequestBody && s.shouldUseExternalStorage(ctx, dataStorage)

	if useExternalStorage {
		// 数据库存放占位 JSON，真实内容写入外部存储成功后才标记 available。
		mut = mut.SetRequestBody([]byte("{}"))
	} else {
		// 直接落库的请求体可在创建成功后标记 available。
		mut = mut.SetRequestBody(requestBodyBytes)
		if storeRequestBody {
			mut = mut.SetRequestBodyAvailability(request.RequestBodyAvailabilityAvailable)
		}
	}

	if dataStorage != nil {
		mut = mut.SetDataStorageID(dataStorage.ID)
	}

	if apiKey, ok := contexts.GetAPIKey(ctx); ok && apiKey != nil {
		mut = mut.SetAPIKeyID(apiKey.ID)
	}

	if trace, ok := contexts.GetTrace(ctx); ok && trace != nil {
		mut = mut.SetTraceID(trace.ID)
	}

	// Create request
	req, err := mut.Save(ctx)
	if err != nil {
		if !useExternalStorage {
			log.Warn(ctx, "Failed to save request body due to error, retrying with placeholder", log.Cause(err))

			mut = mut.
				SetRequestBody(_InvalidRequestBodyJSON).
				SetRequestBodyAvailability(request.RequestBodyAvailabilityUnavailable)

			req, err = mut.Save(ctx)
			if err != nil {
				log.Error(ctx, "Failed to save request even with placeholder", log.Cause(err))
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	// 只有 source=test 记录需要捕获主 Request，供测试结果直接定位详情。
	if contexts.GetSourceOrDefault(ctx, request.SourceAPI) == request.SourceTest {
		relayID := requestIDToRelayID(req.ID)
		// 上下文容器按指针共享，调用方从同一 ctx 可以读取捕获结果。
		contexts.SetTestRequestCapture(ctx, &contexts.TestRequestCapture{RequestID: req.ID, RelayID: relayID})
	}

	// Save request body to external storage if needed
	if useExternalStorage {
		key := GenerateRequestBodyKey(projectID, req.ID)

		err := s.DataStorageService.SaveData(ctx, dataStorage, key, requestBodyBytes)
		if err != nil {
			log.Error(ctx, "Failed to save request body to external storage", log.Cause(err))
			// 继续请求，但保留 unavailable，避免把占位 JSON 当作真实请求体。
		} else {
			if _, updateErr := client.Request.UpdateOneID(req.ID).
				SetRequestBodyAvailability(request.RequestBodyAvailabilityAvailable).
				Save(ctx); updateErr != nil {
				log.Error(ctx, "Failed to update request body availability", log.Cause(updateErr))
			}
		}
	}

	return req, nil
}

// CreateRequestExecution creates a new request execution record.
func (s *RequestService) CreateRequestExecution(
	ctx context.Context,
	channel *Channel,
	modelID string,
	request *ent.Request,
	channelRequest httpclient.Request,
	format llm.APIFormat,
	passThroughApplied bool,
) (*ent.RequestExecution, error) {
	// Decide whether to store the channel request body
	storeRequestBody := true
	if policy, err := s.SystemService.StoragePolicy(ctx); err == nil {
		storeRequestBody = policy.StoreRequestBody
	} else {
		log.Warn(ctx, "Failed to get storage policy, defaulting to store request body", log.Cause(err))
	}

	var (
		requestBodyBytes    objects.JSONRawMessage = []byte("{}")
		requestHeadersBytes objects.JSONRawMessage = []byte("{}")
	)

	if storeRequestBody {
		if len(channelRequest.JSONBody) > 0 {
			requestBodyBytes = channelRequest.JSONBody
		} else {
			b, err := xjson.Marshal(channelRequest.Body)
			if err != nil {
				log.Error(ctx, "Failed to marshal request body", log.Cause(err))
				return nil, err
			}

			requestBodyBytes = b
		}

		if len(channelRequest.Headers) > 0 {
			requestHeadersBytes, _ = xjson.Marshal(httpclient.MaskSensitiveHeaders(channelRequest.Headers))
		}
	}

	client := s.entFromContext(ctx)

	// Get data storage if set on request
	var dataStorage *ent.DataStorage

	if request.DataStorageID != 0 {
		var err error

		dataStorage, err = s.DataStorageService.GetDataStorageByID(ctx, request.DataStorageID)
		if err != nil {
			log.Warn(ctx, "Failed to get data storage for request execution", log.Cause(err))
		}
	}

	// Determine if we should store in database or external storage
	useExternalStorage := storeRequestBody && s.shouldUseExternalStorage(ctx, dataStorage)

	var requestBodyForDB objects.JSONRawMessage
	if useExternalStorage {
		// Set empty JSON for database, actual data will be in external storage
		requestBodyForDB = []byte("{}")
	} else {
		// Store in database
		requestBodyForDB = requestBodyBytes
	}

	requestBodyAvailability := requestexecution.RequestBodyAvailabilityUnavailable
	if storeRequestBody && !useExternalStorage {
		requestBodyAvailability = requestexecution.RequestBodyAvailabilityAvailable
	}
	responseChunksAvailability := requestexecution.ResponseChunksAvailabilityNotApplicable
	if request.Stream {
		responseChunksAvailability = requestexecution.ResponseChunksAvailabilityUnavailable
	}
	mut := client.RequestExecution.Create().
		SetFormat(string(format)).
		SetRequestID(request.ID).
		SetProjectID(request.ProjectID).
		SetChannelID(channel.ID).
		SetModelID(modelID).
		SetRequestBody(requestBodyForDB).
		SetRequestBodyAvailability(requestBodyAvailability).
		SetResponseBodyAvailability(requestexecution.ResponseBodyAvailabilityUnavailable).
		SetResponseChunksAvailability(responseChunksAvailability).
		SetStatus(requestexecution.StatusProcessing).
		SetStream(request.Stream).
		SetRequestHeaders(requestHeadersBytes).
		SetPassThroughApplied(passThroughApplied)

	if channelRequest.URL != "" {
		mut = mut.SetRequestURL(channelRequest.URL)
	}

	// Use the same data storage as the request
	if request.DataStorageID != 0 {
		mut = mut.SetDataStorageID(request.DataStorageID)
	}

	execution, err := mut.Save(ctx)
	if err != nil {
		if useExternalStorage {
			return nil, err
		}

		log.Warn(ctx, "Failed to save execution request body due to error, retrying with placeholder", log.Cause(err))

		mut = mut.
			SetRequestBody(_InvalidRequestBodyJSON).
			SetRequestBodyAvailability(requestexecution.RequestBodyAvailabilityUnavailable)

		execution, err = mut.Save(ctx)
		if err != nil {
			log.Error(ctx, "Failed to save execution request even with placeholder", log.Cause(err))
			return nil, err
		}
	}

	// Save request body to external storage if needed
	if useExternalStorage {
		key := GenerateExecutionRequestBodyKey(request.ProjectID, request.ID, execution.ID)

		err := s.DataStorageService.SaveData(ctx, dataStorage, key, requestBodyBytes)
		if err != nil {
			log.Error(ctx, "Failed to save execution request body to external storage", log.Cause(err))
			// 继续执行，但保留 unavailable，避免占位 JSON 被当作真实请求体。
		} else {
			if _, updateErr := client.RequestExecution.UpdateOneID(execution.ID).
				SetRequestBodyAvailability(requestexecution.RequestBodyAvailabilityAvailable).
				Save(ctx); updateErr != nil {
				log.Error(ctx, "Failed to update execution request body availability", log.Cause(updateErr))
			}
		}
	}

	return execution, nil
}

// LatencyMetrics holds latency metrics for a request.
type LatencyMetrics struct {
	LatencyMs           *int64
	FirstTokenLatencyMs *int64
	ReasoningDurationMs *int64
}

// UpdateRequestCompleted updates request status to completed with response body.
func (s *RequestService) UpdateRequestCompleted(
	ctx context.Context,
	requestID int,
	externalId string,
	responseBody any,
	metrics *LatencyMetrics,
) error {
	// Decide whether to store the final response body
	storeResponseBody := true
	if policy, err := s.SystemService.StoragePolicy(ctx); err == nil {
		storeResponseBody = policy.StoreResponseBody
	} else {
		log.Warn(ctx, "Failed to get storage policy, defaulting to store response body", log.Cause(err))
	}

	client := s.entFromContext(ctx)

	// Get the request to check data storage
	req, err := client.Request.Get(ctx, requestID)
	if err != nil {
		log.Error(ctx, "Failed to get request", log.Cause(err))
		return err
	}

	// Get data storage if set
	var dataStorage *ent.DataStorage
	if req.DataStorageID != 0 {
		dataStorage, err = s.DataStorageService.GetDataStorageByID(ctx, req.DataStorageID)
		if err != nil {
			log.Warn(ctx, "Failed to get data storage", log.Cause(err))
		}
	}

	responseBodyAvailability := request.ResponseBodyAvailabilityUnavailable
	upd := client.Request.UpdateOneID(requestID).
		SetStatus(request.StatusCompleted).
		SetExternalID(externalId)

	// Set latency metrics if provided
	if metrics != nil {
		if metrics.LatencyMs != nil {
			upd = upd.SetMetricsLatencyMs(*metrics.LatencyMs)
		}

		if metrics.FirstTokenLatencyMs != nil {
			upd = upd.SetMetricsFirstTokenLatencyMs(*metrics.FirstTokenLatencyMs)
		}

		if metrics.ReasoningDurationMs != nil {
			upd = upd.SetMetricsReasoningDurationMs(*metrics.ReasoningDurationMs)
		}
	}

	if storeResponseBody {
		responseBodyBytes, err := xjson.Marshal(responseBody)
		if err != nil {
			log.Error(ctx, "Failed to serialize response body", log.Cause(err))
			return err
		}

		// Check if we should use external storage
		if s.shouldUseExternalStorage(ctx, dataStorage) {
			// Save to external storage
			key := GenerateResponseBodyKey(req.ProjectID, requestID)

			err := s.DataStorageService.SaveData(ctx, dataStorage, key, responseBodyBytes)
			if err != nil {
				log.Error(ctx, "Failed to save response body to external storage", log.Cause(err))
				// Continue anyway, but keep unavailable.
			} else {
				responseBodyAvailability = request.ResponseBodyAvailabilityAvailable
			}
		} else {
			// Store in database
			upd = upd.SetResponseBody(responseBodyBytes)
			responseBodyAvailability = request.ResponseBodyAvailabilityAvailable
		}
	}
	upd = upd.SetResponseBodyAvailability(responseBodyAvailability)

	_, err = upd.Save(ctx)
	if err != nil {
		log.Error(ctx, "Failed to update request status to completed", log.Cause(err))
		return err
	}

	return nil
}

// UpdateRequestCompletedWithAudio marks a request completed and persists a binary audio
// payload (TTS) to external storage when configured.
//
// The audio bytes are never stored in the database column: responseBody carries a compact
// metadata placeholder, and the raw audio is saved to the request's external DataStorage
// (when one is configured and non-primary), tracked via the content_storage_* fields,
// mirroring how video artifacts are stored.
func (s *RequestService) UpdateRequestCompletedWithAudio(
	ctx context.Context,
	requestID int,
	externalId string,
	responseBody any,
	audio []byte,
	filename string,
	metrics *LatencyMetrics,
) error {
	// Decide whether to store the final response body metadata.
	storeResponseBody := true
	if policy, err := s.SystemService.StoragePolicy(ctx); err == nil {
		storeResponseBody = policy.StoreResponseBody
	} else {
		log.Warn(ctx, "Failed to get storage policy, defaulting to store response body", log.Cause(err))
	}

	client := s.entFromContext(ctx)

	req, err := client.Request.Get(ctx, requestID)
	if err != nil {
		log.Error(ctx, "Failed to get request", log.Cause(err))
		return err
	}

	var dataStorage *ent.DataStorage
	if req.DataStorageID != 0 {
		dataStorage, err = s.DataStorageService.GetDataStorageByID(ctx, req.DataStorageID)
		if err != nil {
			log.Warn(ctx, "Failed to get data storage", log.Cause(err))
		}
	}

	responseBodyAvailability := request.ResponseBodyAvailabilityUnavailable
	upd := client.Request.UpdateOneID(requestID).
		SetStatus(request.StatusCompleted).
		SetExternalID(externalId)

	if metrics != nil {
		if metrics.LatencyMs != nil {
			upd = upd.SetMetricsLatencyMs(*metrics.LatencyMs)
		}

		if metrics.FirstTokenLatencyMs != nil {
			upd = upd.SetMetricsFirstTokenLatencyMs(*metrics.FirstTokenLatencyMs)
		}

		if metrics.ReasoningDurationMs != nil {
			upd = upd.SetMetricsReasoningDurationMs(*metrics.ReasoningDurationMs)
		}
	}

	if storeResponseBody {
		responseBodyBytes, err := xjson.Marshal(responseBody)
		if err != nil {
			log.Error(ctx, "Failed to serialize response body", log.Cause(err))
			return err
		}

		if s.shouldUseExternalStorage(ctx, dataStorage) {
			key := GenerateResponseBodyKey(req.ProjectID, requestID)
			if err := s.DataStorageService.SaveData(ctx, dataStorage, key, responseBodyBytes); err != nil {
				log.Error(ctx, "Failed to save response body to external storage", log.Cause(err))
			} else {
				responseBodyAvailability = request.ResponseBodyAvailabilityAvailable
			}
		} else {
			upd = upd.SetResponseBody(responseBodyBytes)
			responseBodyAvailability = request.ResponseBodyAvailabilityAvailable
		}
	}
	upd = upd.SetResponseBodyAvailability(responseBodyAvailability)

	// Persist the binary audio to external storage when one is configured.
	if len(audio) > 0 && s.shouldUseExternalStorage(ctx, dataStorage) {
		key := GenerateAudioKey(req.ProjectID, requestID, filename)
		if err := s.DataStorageService.SaveData(ctx, dataStorage, key, audio); err != nil {
			log.Error(ctx, "Failed to save audio to external storage", log.Cause(err))
		} else {
			upd = upd.
				SetContentSaved(true).
				SetContentStorageID(dataStorage.ID).
				SetContentStorageKey(key).
				SetContentSavedAt(time.Now().UTC())
		}
	}

	_, err = upd.Save(ctx)
	if err != nil {
		log.Error(ctx, "Failed to update audio request status to completed", log.Cause(err))
		return err
	}

	return nil
}

// UpdateRequestStatusExternalIDAndResponseBody updates request status/external_id and optionally persists response body.
// It is intended for non-pipeline async task flows where task status is polled later.
func (s *RequestService) UpdateRequestStatusExternalIDAndResponseBody(
	ctx context.Context,
	requestID int,
	status request.Status,
	externalId string,
	responseBody any,
	metrics *LatencyMetrics,
) error {
	// Decide whether to store the final response body
	storeResponseBody := true
	if policy, err := s.SystemService.StoragePolicy(ctx); err == nil {
		storeResponseBody = policy.StoreResponseBody
	} else {
		log.Warn(ctx, "Failed to get storage policy, defaulting to store response body", log.Cause(err))
	}

	client := s.entFromContext(ctx)

	// Get the request to check data storage
	req, err := client.Request.Get(ctx, requestID)
	if err != nil {
		log.Error(ctx, "Failed to get request", log.Cause(err))
		return err
	}

	// Get data storage if set
	var dataStorage *ent.DataStorage
	if req.DataStorageID != 0 {
		dataStorage, err = s.DataStorageService.GetDataStorageByID(ctx, req.DataStorageID)
		if err != nil {
			log.Warn(ctx, "Failed to get data storage", log.Cause(err))
		}
	}

	responseBodyAvailability := request.ResponseBodyAvailabilityUnavailable
	upd := client.Request.UpdateOneID(requestID).
		SetStatus(status).
		SetExternalID(externalId)

	// Set latency metrics if provided
	if metrics != nil {
		if metrics.LatencyMs != nil {
			upd = upd.SetMetricsLatencyMs(*metrics.LatencyMs)
		}

		if metrics.FirstTokenLatencyMs != nil {
			upd = upd.SetMetricsFirstTokenLatencyMs(*metrics.FirstTokenLatencyMs)
		}

		if metrics.ReasoningDurationMs != nil {
			upd = upd.SetMetricsReasoningDurationMs(*metrics.ReasoningDurationMs)
		}
	}

	if storeResponseBody {
		responseBodyBytes, err := xjson.Marshal(responseBody)
		if err != nil {
			log.Error(ctx, "Failed to serialize response body", log.Cause(err))
			return err
		}

		// Check if we should use external storage
		if s.shouldUseExternalStorage(ctx, dataStorage) {
			// Save to external storage
			key := GenerateResponseBodyKey(req.ProjectID, requestID)

			err := s.DataStorageService.SaveData(ctx, dataStorage, key, responseBodyBytes)
			if err != nil {
				log.Error(ctx, "Failed to save response body to external storage", log.Cause(err))
				// Continue anyway, but keep unavailable.
			} else {
				responseBodyAvailability = request.ResponseBodyAvailabilityAvailable
			}
		} else {
			// Store in database
			upd = upd.SetResponseBody(responseBodyBytes)
			responseBodyAvailability = request.ResponseBodyAvailabilityAvailable
		}
	}
	upd = upd.SetResponseBodyAvailability(responseBodyAvailability)

	_, err = upd.Save(ctx)
	if err != nil {
		log.Error(ctx, "Failed to update request status", log.Cause(err))
		return err
	}

	return nil
}

// UpdateRequestExecutionCompleted updates request execution status to completed with response body.
func (s *RequestService) UpdateRequestExecutionCompleted(
	ctx context.Context,
	executionID int,
	externalId string,
	responseBody any,
	metrics *LatencyMetrics,
) error {
	// Decide whether to store the final response body for execution
	storeResponseBody := true
	if policy, err := s.SystemService.StoragePolicy(ctx); err == nil {
		storeResponseBody = policy.StoreResponseBody
	} else {
		log.Warn(ctx, "Failed to get storage policy, defaulting to store response body", log.Cause(err))
	}

	client := s.entFromContext(ctx)

	// Get the execution to check data storage
	execution, err := client.RequestExecution.Get(ctx, executionID)
	if err != nil {
		log.Error(ctx, "Failed to get request execution", log.Cause(err))
		return err
	}

	// Get data storage if set
	var dataStorage *ent.DataStorage
	if execution.DataStorageID != 0 {
		dataStorage, err = client.DataStorage.Get(ctx, execution.DataStorageID)
		if err != nil {
			log.Warn(ctx, "Failed to get data storage", log.Cause(err))
		}
	}

	responseBodyAvailability := requestexecution.ResponseBodyAvailabilityUnavailable
	upd := client.RequestExecution.UpdateOneID(executionID).
		SetStatus(requestexecution.StatusCompleted).
		SetExternalID(externalId)

	// Set latency metrics if provided
	if metrics != nil {
		if metrics.LatencyMs != nil {
			upd = upd.SetMetricsLatencyMs(*metrics.LatencyMs)
		}

		if metrics.FirstTokenLatencyMs != nil {
			upd = upd.SetMetricsFirstTokenLatencyMs(*metrics.FirstTokenLatencyMs)
		}

		if metrics.ReasoningDurationMs != nil {
			upd = upd.SetMetricsReasoningDurationMs(*metrics.ReasoningDurationMs)
		}
	}

	if storeResponseBody {
		responseBodyBytes, err := xjson.Marshal(responseBody)
		if err != nil {
			return err
		}

		// Check if we should use external storage
		if s.shouldUseExternalStorage(ctx, dataStorage) {
			// Save to external storage
			key := GenerateExecutionResponseBodyKey(execution.ProjectID, execution.RequestID, executionID)

			err := s.DataStorageService.SaveData(ctx, dataStorage, key, responseBodyBytes)
			if err != nil {
				log.Error(ctx, "Failed to save execution response body to external storage", log.Cause(err))
			} else {
				responseBodyAvailability = requestexecution.ResponseBodyAvailabilityAvailable
			}
		} else {
			// Store in database
			upd = upd.SetResponseBody(responseBodyBytes)
			responseBodyAvailability = requestexecution.ResponseBodyAvailabilityAvailable
		}
	}
	upd = upd.SetResponseBodyAvailability(responseBodyAvailability)

	_, err = upd.Save(ctx)
	if err != nil {
		log.Error(ctx, "Failed to update request execution status to completed", log.Cause(err))
		return err
	}

	return nil
}

// UpdateRequestExecutionCanceled updates request execution status to canceled with error message.
func (s *RequestService) UpdateRequestExecutionCanceled(
	ctx context.Context,
	executionID int,
	errorMsg string,
) error {
	return s.UpdateRequestExecutionStatus(ctx, executionID, requestexecution.StatusCanceled, errorMsg, nil)
}

// ExecutionErrorInfo holds error details for a failed request execution.
type ExecutionErrorInfo struct {
	StatusCode *int
}

// UpdateRequestExecutionFailed updates request execution status to failed with error message and optional error details.
func (s *RequestService) UpdateRequestExecutionFailed(
	ctx context.Context,
	executionID int,
	errorMsg string,
	errorInfo *ExecutionErrorInfo,
) error {
	return s.UpdateRequestExecutionStatus(ctx, executionID, requestexecution.StatusFailed, errorMsg, errorInfo)
}

// UpdateRequestExecutionStatus updates request execution status to the provided value (e.g., canceled or failed), with optional error message.
func (s *RequestService) UpdateRequestExecutionStatus(
	ctx context.Context,
	executionID int,
	status requestexecution.Status,
	errorMsg string,
	errorInfo *ExecutionErrorInfo,
) error {
	client := s.entFromContext(ctx)

	upd := client.RequestExecution.UpdateOneID(executionID).
		SetStatus(status)
	if errorMsg != "" {
		upd = upd.SetErrorMessage(errorMsg)
	}

	if errorInfo != nil && errorInfo.StatusCode != nil {
		upd = upd.SetResponseStatusCode(*errorInfo.StatusCode)
	}

	_, err := upd.Save(ctx)
	if err != nil {
		log.Error(ctx, "Failed to update request execution status", log.Cause(err), log.Any("status", status))
		return err
	}

	return nil
}

// UpdateRequestExecutionStatusFromError updates request execution status based on error type and sets error message.
func (s *RequestService) UpdateRequestExecutionStatusFromError(ctx context.Context, executionID int, rawErr error) error {
	status := requestexecution.StatusFailed
	if errors.Is(rawErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		status = requestexecution.StatusCanceled
	}

	return s.UpdateRequestExecutionStatus(ctx, executionID, status, rawErr.Error(), nil)
}

type jsonStreamEvent struct {
	LastEventID string          `json:"last_event_id,omitempty"`
	Type        string          `json:"event"`
	Data        json.RawMessage `json:"data"`
}

type binaryStreamChunkSummary struct {
	Object      string `json:"object"`
	ContentType string `json:"content_type"`
	Bytes       int    `json:"bytes"`
}

func isBinaryStreamChunk(chunk *httpclient.StreamEvent) bool {
	if chunk == nil {
		return false
	}

	eventType := strings.ToLower(strings.TrimSpace(chunk.Type))

	return strings.HasPrefix(eventType, "audio/") || eventType == "application/octet-stream"
}

func shouldSkipStoredStreamChunk(chunk *httpclient.StreamEvent) bool {
	return chunk == nil ||
		(!isBinaryStreamChunk(chunk) && bytes.Equal(chunk.Data, llm.DoneStreamEvent.Data)) ||
		chunk.Type == httpclient.BinaryStreamDoneEventType
}

func marshalStreamEventForStorage(chunk *httpclient.StreamEvent) (objects.JSONRawMessage, error) {
	data := json.RawMessage(chunk.Data)
	if isBinaryStreamChunk(chunk) {
		// Prefer chunk.Size, which is set when the persistence layer summarized the
		// raw audio chunk to avoid buffering audio bytes in memory.
		byteCount := len(chunk.Data)
		if byteCount == 0 {
			byteCount = chunk.Size
		}

		var err error
		data, err = json.Marshal(binaryStreamChunkSummary{
			Object:      "binary.stream_chunk",
			ContentType: strings.TrimSpace(chunk.Type),
			Bytes:       byteCount,
		})
		if err != nil {
			return nil, err
		}
	}

	return xjson.Marshal(jsonStreamEvent{
		LastEventID: chunk.LastEventID,
		Type:        chunk.Type,
		Data:        data,
	})
}

// SaveRequestExecutionChunks saves all response chunks to request execution at once.
// Only stores chunks if the system StoreChunks setting is enabled.
func (s *RequestService) SaveRequestExecutionChunks(
	ctx context.Context,
	executionID int,
	chunks []*httpclient.StreamEvent,
) error {
	if len(chunks) == 0 {
		return nil
	}

	// Check if chunk storage is enabled
	storeChunks, err := s.SystemService.StoreChunks(ctx)
	if err != nil {
		log.Warn(ctx, "Failed to get StoreChunks setting, defaulting to false", log.Cause(err))

		storeChunks = false
	}

	// Only store chunks if enabled
	if !storeChunks {
		return nil
	}

	// Convert chunks to JSON format, filtering out done events
	chunkBytes := make([]objects.JSONRawMessage, 0, len(chunks))

	for _, chunk := range chunks {
		if shouldSkipStoredStreamChunk(chunk) {
			continue
		}

		b, err := marshalStreamEventForStorage(chunk)
		if err != nil {
			log.Warn(ctx, "Failed to marshal chunk, skipping", log.Cause(err))

			continue
		}

		chunkBytes = append(chunkBytes, b)
	}

	client := s.entFromContext(ctx)

	// Get the execution to check data storage
	execution, err := client.RequestExecution.Get(ctx, executionID)
	if err != nil {
		return fmt.Errorf("failed to get request execution: %w", err)
	}
	if !execution.Stream {
		_, err = client.RequestExecution.UpdateOneID(executionID).
			SetResponseChunksAvailability(requestexecution.ResponseChunksAvailabilityNotApplicable).
			Save(ctx)
		return err
	}

	// Get data storage if set
	var dataStorage *ent.DataStorage
	if execution.DataStorageID != 0 {
		dataStorage, err = client.DataStorage.Get(ctx, execution.DataStorageID)
		if err != nil {
			log.Warn(ctx, "Failed to get data storage", log.Cause(err))
		}
	}

	// Check if we should use external storage
	if s.shouldUseExternalStorage(ctx, dataStorage) {
		key := GenerateExecutionResponseChunksKey(execution.ProjectID, execution.RequestID, executionID)

		allChunksBytes, err := json.Marshal(chunkBytes)
		if err != nil {
			return fmt.Errorf("failed to marshal all chunks: %w", err)
		}

		err = s.DataStorageService.SaveData(ctx, dataStorage, key, allChunksBytes)
		if err != nil {
			return fmt.Errorf("failed to save chunks to external storage: %w", err)
		}
		_, err = client.RequestExecution.UpdateOneID(executionID).
			SetResponseChunksAvailability(requestexecution.ResponseChunksAvailabilityAvailable).
			Save(ctx)
	} else {
		// Store in database
		_, err = client.RequestExecution.UpdateOneID(executionID).
			SetResponseChunks(chunkBytes).
			SetResponseChunksAvailability(requestexecution.ResponseChunksAvailabilityAvailable).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to save response chunks: %w", err)
		}
	}

	return nil
}

// SaveRequestChunks saves all response chunks to request at once.
// Only stores chunks if the system StoreChunks setting is enabled.
func (s *RequestService) SaveRequestChunks(
	ctx context.Context,
	requestID int,
	chunks []*httpclient.StreamEvent,
) error {
	if len(chunks) == 0 {
		return nil
	}

	storeChunks, err := s.SystemService.StoreChunks(ctx)
	if err != nil {
		log.Warn(ctx, "Failed to get StoreChunks setting, defaulting to false", log.Cause(err))

		storeChunks = false
	}

	// Only store chunks if enabled
	if !storeChunks {
		return nil
	}

	// Convert chunks to JSON format, filtering out done events
	chunkBytes := make([]objects.JSONRawMessage, 0, len(chunks))

	for _, chunk := range chunks {
		if shouldSkipStoredStreamChunk(chunk) {
			continue
		}

		b, err := marshalStreamEventForStorage(chunk)
		if err != nil {
			log.Warn(ctx, "Failed to marshal chunk, skipping", log.Cause(err))

			continue
		}

		chunkBytes = append(chunkBytes, b)
	}

	client := s.entFromContext(ctx)

	// Get the request to check data storage
	req, err := client.Request.Get(ctx, requestID)
	if err != nil {
		return fmt.Errorf("failed to get request: %w", err)
	}
	if !req.Stream {
		_, err = client.Request.UpdateOneID(requestID).
			SetResponseChunksAvailability(request.ResponseChunksAvailabilityNotApplicable).
			Save(ctx)
		return err
	}

	// Get data storage if set
	var dataStorage *ent.DataStorage
	if req.DataStorageID != 0 {
		dataStorage, err = client.DataStorage.Get(ctx, req.DataStorageID)
		if err != nil {
			log.Warn(ctx, "Failed to get data storage", log.Cause(err))
		}
	}

	// Check if we should use external storage
	if s.shouldUseExternalStorage(ctx, dataStorage) {
		key := GenerateResponseChunksKey(req.ProjectID, requestID)

		allChunksBytes, err := json.Marshal(chunkBytes)
		if err != nil {
			return fmt.Errorf("failed to marshal all chunks: %w", err)
		}

		err = s.DataStorageService.SaveData(ctx, dataStorage, key, allChunksBytes)
		if err != nil {
			return fmt.Errorf("failed to save chunks to external storage: %w", err)
		}
		_, err = client.Request.UpdateOneID(requestID).
			SetResponseChunksAvailability(request.ResponseChunksAvailabilityAvailable).
			Save(ctx)
	} else {
		// Store in database
		_, err = client.Request.UpdateOneID(requestID).
			SetResponseChunks(chunkBytes).
			SetResponseChunksAvailability(request.ResponseChunksAvailabilityAvailable).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to save response chunks: %w", err)
		}
	}

	return nil
}

// MarkRequestCanceled updates request status to canceled.
func (s *RequestService) MarkRequestCanceled(ctx context.Context, requestID int) error {
	return s.UpdateRequestStatus(ctx, requestID, request.StatusCanceled)
}

// MarkRequestFailed updates request status to failed.
func (s *RequestService) MarkRequestFailed(ctx context.Context, requestID int) error {
	return s.UpdateRequestStatus(ctx, requestID, request.StatusFailed)
}

// UpdateRequestStatus updates request status to the provided value (e.g., canceled or failed).
func (s *RequestService) UpdateRequestStatus(ctx context.Context, requestID int, status request.Status) error {
	client := s.entFromContext(ctx)

	_, err := client.Request.UpdateOneID(requestID).
		SetStatus(status).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update request status: %w", err)
	}

	return nil
}

// UpdateRequestStatusFromError updates request status based on error type: canceled if context canceled, otherwise failed.
func (s *RequestService) UpdateRequestStatusFromError(ctx context.Context, requestID int, rawErr error) error {
	if errors.Is(rawErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return s.UpdateRequestStatus(ctx, requestID, request.StatusCanceled)
	}

	return s.UpdateRequestStatus(ctx, requestID, request.StatusFailed)
}

// cancelStaleRecords updates records older than maxAge to canceled status.
func (s *RequestService) cancelStaleRecords(
	ctx context.Context,
	maxAge time.Duration,
	entityName string,
	updateFn func(ctx context.Context, cutoff time.Time) (int, error),
) error {
	cutoff := time.Now().UTC().Add(-maxAge)
	return authz.RunWithSystemBypassVoid(ctx, "cleanup-"+entityName, func(ctx context.Context) error {
		count, err := updateFn(ctx, cutoff)
		if err != nil {
			return fmt.Errorf("failed to cancel stale %s: %w", entityName, err)
		}
		if count > 0 {
			log.Info(ctx, "canceled stale processing records",
				log.String("entity", entityName),
				log.Int("count", count),
				log.Duration("maxAge", maxAge))
		}
		return nil
	})
}

// maxProcessingDuration defines how long a record can be in "processing" state.
// Records exceeding this are considered stuck and will be canceled on startup.
const maxProcessingDuration = 1 * time.Hour

func (s *RequestService) ClearStaleProcessingOnStartup(ctx context.Context) error {
	var errs []error

	if err := s.cancelStaleRecords(ctx, maxProcessingDuration, "requests", func(ctx context.Context, cutoff time.Time) (int, error) {
		return s.entFromContext(ctx).Request.Update().
			Where(
				request.StatusEQ(request.StatusProcessing),
				request.CreatedAtLT(cutoff),
			).
			SetStatus(request.StatusCanceled).
			Save(ctx)
	}); err != nil {
		errs = append(errs, err)
	}

	if err := s.cancelStaleRecords(ctx, maxProcessingDuration, "executions", func(ctx context.Context, cutoff time.Time) (int, error) {
		return s.entFromContext(ctx).RequestExecution.Update().
			Where(
				requestexecution.StatusEQ(requestexecution.StatusProcessing),
				requestexecution.CreatedAtLT(cutoff),
			).
			SetStatus(requestexecution.StatusCanceled).
			Save(ctx)
	}); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("startup cleanup failed: %w", errors.Join(errs...))
	}
	return nil
}

// UpdateRequestChannelID updates request with channel ID after channel selection.
func (s *RequestService) UpdateRequestChannelID(ctx context.Context, requestID int, channelID int) error {
	client := s.entFromContext(ctx)

	req, err := client.Request.UpdateOneID(requestID).
		SetChannelID(channelID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update request channel ID: %w", err)
	}

	s.cachePreviousChannelForRequest(ctx, req)

	return nil
}

func hasRequestContentJSONEvidence(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}

	switch string(trimmed) {
	case "null", "{}", "[]", `{"message":"invalid text"}`:
		return false
	default:
		return json.Valid(trimmed)
	}
}

func hasRequestContentChunksEvidence(chunks []objects.JSONRawMessage) bool {
	for _, chunk := range chunks {
		if hasRequestContentJSONEvidence(chunk) {
			return true
		}
	}

	return false
}

func (s *RequestService) backfillRequestBodyAvailability(ctx context.Context, req *ent.Request, raw []byte) {
	if req.RequestBodyAvailability != request.RequestBodyAvailabilityUnknown || !hasRequestContentJSONEvidence(raw) {
		return
	}

	bypassCtx := authz.WithSystemBypass(ctx, "request-body-availability-backfill")
	if _, err := s.entFromContext(bypassCtx).Request.UpdateOneID(req.ID).
		SetRequestBodyAvailability(request.RequestBodyAvailabilityAvailable).
		Save(bypassCtx); err != nil {
		log.Warn(ctx, "Failed to backfill request body availability", log.Cause(err), log.Int("request_id", req.ID))
		return
	}

	req.RequestBodyAvailability = request.RequestBodyAvailabilityAvailable
}

func (s *RequestService) backfillResponseBodyAvailability(ctx context.Context, req *ent.Request, raw []byte) {
	if req.ResponseBodyAvailability != request.ResponseBodyAvailabilityUnknown || !hasRequestContentJSONEvidence(raw) {
		return
	}

	bypassCtx := authz.WithSystemBypass(ctx, "response-body-availability-backfill")
	if _, err := s.entFromContext(bypassCtx).Request.UpdateOneID(req.ID).
		SetResponseBodyAvailability(request.ResponseBodyAvailabilityAvailable).
		Save(bypassCtx); err != nil {
		log.Warn(ctx, "Failed to backfill response body availability", log.Cause(err), log.Int("request_id", req.ID))
		return
	}

	req.ResponseBodyAvailability = request.ResponseBodyAvailabilityAvailable
}

func (s *RequestService) backfillRequestChunksAvailability(ctx context.Context, req *ent.Request, chunks []objects.JSONRawMessage) {
	if req.ResponseChunksAvailability != request.ResponseChunksAvailabilityUnknown || !hasRequestContentChunksEvidence(chunks) {
		return
	}

	bypassCtx := authz.WithSystemBypass(ctx, "request-chunks-availability-backfill")
	if _, err := s.entFromContext(bypassCtx).Request.UpdateOneID(req.ID).
		SetResponseChunksAvailability(request.ResponseChunksAvailabilityAvailable).
		Save(bypassCtx); err != nil {
		log.Warn(ctx, "Failed to backfill request chunks availability", log.Cause(err), log.Int("request_id", req.ID))
		return
	}

	req.ResponseChunksAvailability = request.ResponseChunksAvailabilityAvailable
}

func (s *RequestService) backfillExecutionRequestBodyAvailability(ctx context.Context, exec *ent.RequestExecution, raw []byte) {
	if exec.RequestBodyAvailability != requestexecution.RequestBodyAvailabilityUnknown || !hasRequestContentJSONEvidence(raw) {
		return
	}

	bypassCtx := authz.WithSystemBypass(ctx, "execution-request-body-availability-backfill")
	if _, err := s.entFromContext(bypassCtx).RequestExecution.UpdateOneID(exec.ID).
		SetRequestBodyAvailability(requestexecution.RequestBodyAvailabilityAvailable).
		Save(bypassCtx); err != nil {
		log.Warn(ctx, "Failed to backfill execution request body availability", log.Cause(err), log.Int("request_execution_id", exec.ID))
		return
	}

	exec.RequestBodyAvailability = requestexecution.RequestBodyAvailabilityAvailable
}

func (s *RequestService) backfillExecutionResponseBodyAvailability(ctx context.Context, exec *ent.RequestExecution, raw []byte) {
	if exec.ResponseBodyAvailability != requestexecution.ResponseBodyAvailabilityUnknown || !hasRequestContentJSONEvidence(raw) {
		return
	}

	bypassCtx := authz.WithSystemBypass(ctx, "execution-response-body-availability-backfill")
	if _, err := s.entFromContext(bypassCtx).RequestExecution.UpdateOneID(exec.ID).
		SetResponseBodyAvailability(requestexecution.ResponseBodyAvailabilityAvailable).
		Save(bypassCtx); err != nil {
		log.Warn(ctx, "Failed to backfill execution response body availability", log.Cause(err), log.Int("request_execution_id", exec.ID))
		return
	}

	exec.ResponseBodyAvailability = requestexecution.ResponseBodyAvailabilityAvailable
}

func (s *RequestService) backfillExecutionChunksAvailability(ctx context.Context, exec *ent.RequestExecution, chunks []objects.JSONRawMessage) {
	if exec.ResponseChunksAvailability != requestexecution.ResponseChunksAvailabilityUnknown || !hasRequestContentChunksEvidence(chunks) {
		return
	}

	bypassCtx := authz.WithSystemBypass(ctx, "execution-chunks-availability-backfill")
	if _, err := s.entFromContext(bypassCtx).RequestExecution.UpdateOneID(exec.ID).
		SetResponseChunksAvailability(requestexecution.ResponseChunksAvailabilityAvailable).
		Save(bypassCtx); err != nil {
		log.Warn(ctx, "Failed to backfill execution chunks availability", log.Cause(err), log.Int("request_execution_id", exec.ID))
		return
	}

	exec.ResponseChunksAvailability = requestexecution.ResponseChunksAvailabilityAvailable
}

// ResolveRequestBodyAvailability 在读取旧记录时补充可确认的请求体证据。
func (s *RequestService) ResolveRequestBodyAvailability(ctx context.Context, req *ent.Request) request.RequestBodyAvailability {
	if req != nil && req.RequestBodyAvailability == request.RequestBodyAvailabilityUnknown && s.DataStorageService != nil {
		_, _ = s.LoadRequestBody(ctx, req)
	}
	if req == nil {
		return request.RequestBodyAvailabilityUnknown
	}
	return req.RequestBodyAvailability
}

// ResolveResponseBodyAvailability 在读取旧记录时补充可确认的响应体证据。
func (s *RequestService) ResolveResponseBodyAvailability(ctx context.Context, req *ent.Request) request.ResponseBodyAvailability {
	if req != nil && req.ResponseBodyAvailability == request.ResponseBodyAvailabilityUnknown && s.DataStorageService != nil {
		_, _ = s.LoadResponseBody(ctx, req)
	}
	if req == nil {
		return request.ResponseBodyAvailabilityUnknown
	}
	return req.ResponseBodyAvailability
}

// ResolveExecutionRequestBodyAvailability 在读取旧记录时补充可确认的 execution 请求体证据。
func (s *RequestService) ResolveExecutionRequestBodyAvailability(ctx context.Context, exec *ent.RequestExecution) requestexecution.RequestBodyAvailability {
	if exec != nil && exec.RequestBodyAvailability == requestexecution.RequestBodyAvailabilityUnknown && s.DataStorageService != nil {
		_, _ = s.LoadRequestExecutionRequestBody(ctx, exec)
	}
	if exec == nil {
		return requestexecution.RequestBodyAvailabilityUnknown
	}
	return exec.RequestBodyAvailability
}

// ResolveExecutionResponseBodyAvailability 在读取旧记录时补充可确认的 execution 响应体证据。
func (s *RequestService) ResolveExecutionResponseBodyAvailability(ctx context.Context, exec *ent.RequestExecution) requestexecution.ResponseBodyAvailability {
	if exec != nil && exec.ResponseBodyAvailability == requestexecution.ResponseBodyAvailabilityUnknown && s.DataStorageService != nil {
		_, _ = s.LoadRequestExecutionResponseBody(ctx, exec)
	}
	if exec == nil {
		return requestexecution.ResponseBodyAvailabilityUnknown
	}
	return exec.ResponseBodyAvailability
}

// LoadRequestBody returns the stored request body, loading from external storage when necessary.
func (s *RequestService) LoadRequestBody(ctx context.Context, req *ent.Request) (objects.JSONRawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	dataStorage, err := s.getDataStorage(ctx, req.DataStorageID)
	if err != nil {
		log.Warn(ctx, "Failed to get data storage for request body", log.Cause(err), log.Int("request_id", req.ID))
		return xjson.EmptyJSONRawMessage, nil
	}

	if !s.shouldUseExternalStorage(ctx, dataStorage) {
		if req.RequestBody == nil {
			return xjson.EmptyJSONRawMessage, nil
		}

		s.backfillRequestBodyAvailability(ctx, req, req.RequestBody)
		return req.RequestBody, nil
	}

	key := GenerateRequestBodyKey(req.ProjectID, req.ID)

	data, err := s.DataStorageService.LoadData(ctx, dataStorage, key)
	if err != nil {
		return xjson.EmptyJSONRawMessage, nil
	}

	if json.Valid(data) {
		s.backfillRequestBodyAvailability(ctx, req, data)
		return objects.JSONRawMessage(data), nil
	}

	return xjson.EmptyJSONRawMessage, nil
}

// LoadResponseBody returns the request response body, loading from external storage when necessary.
func (s *RequestService) LoadResponseBody(ctx context.Context, req *ent.Request) (objects.JSONRawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	// Only load response body if request is completed
	if req.Status != request.StatusCompleted {
		return xjson.EmptyJSONRawMessage, nil
	}

	dataStorage, err := s.getDataStorage(ctx, req.DataStorageID)
	if err != nil {
		log.Warn(ctx, "Failed to get data storage for request response body", log.Cause(err), log.Int("request_id", req.ID))
		return xjson.EmptyJSONRawMessage, nil
	}

	if !s.shouldUseExternalStorage(ctx, dataStorage) {
		if req.ResponseBody == nil {
			return xjson.EmptyJSONRawMessage, nil
		}

		s.backfillResponseBodyAvailability(ctx, req, req.ResponseBody)
		return req.ResponseBody, nil
	}

	key := GenerateResponseBodyKey(req.ProjectID, req.ID)

	data, err := s.DataStorageService.LoadData(ctx, dataStorage, key)
	if err != nil {
		return xjson.EmptyJSONRawMessage, nil
	}

	if json.Valid(data) {
		s.backfillResponseBodyAvailability(ctx, req, data)
		return objects.JSONRawMessage(data), nil
	}

	return xjson.EmptyJSONRawMessage, nil
}

// IsRequestResponseChunksLive 判断 Request 是否存在可读取的临时内存流预览。
func (s *RequestService) IsRequestResponseChunksLive(req *ent.Request) bool {
	if req == nil || !req.Stream || req.Status != request.StatusProcessing || s.LiveStreamRegistry == nil {
		return false
	}

	buffer := s.LiveStreamRegistry.GetRequestBuffer(req.ID)
	return buffer != nil && !buffer.IsClosed() && buffer.Len() > 0
}

// IsRequestExecutionResponseChunksLive 判断 RequestExecution 是否存在可读取的临时内存流预览。
func (s *RequestService) IsRequestExecutionResponseChunksLive(exec *ent.RequestExecution) bool {
	if exec == nil || !exec.Stream || exec.Status != requestexecution.StatusProcessing || s.LiveStreamRegistry == nil {
		return false
	}

	buffer := s.LiveStreamRegistry.GetExecutionBuffer(exec.ID)
	return buffer != nil && !buffer.IsClosed() && buffer.Len() > 0
}

// LoadResponseChunks returns the request response chunks, loading from external storage when necessary.
func (s *RequestService) LoadResponseChunks(ctx context.Context, req *ent.Request) ([]objects.JSONRawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	// Live preview for active streaming requests
	if req.Stream && req.Status == request.StatusProcessing && s.LiveStreamRegistry != nil {
		chunks := s.LiveStreamRegistry.GetRequestChunks(req.ID)
		return chunks, nil
	}
	// Only load response chunks if request is completed and streaming.
	if !req.Stream || req.Status != request.StatusCompleted {
		return []objects.JSONRawMessage{}, nil
	}

	dataStorage, err := s.getDataStorage(ctx, req.DataStorageID)
	if err != nil {
		log.Warn(ctx, "Failed to get data storage for request response chunks", log.Cause(err), log.Int("request_id", req.ID))
		return []objects.JSONRawMessage{}, nil
	}

	if !s.shouldUseExternalStorage(ctx, dataStorage) {
		s.backfillRequestChunksAvailability(ctx, req, req.ResponseChunks)
		return req.ResponseChunks, nil
	}

	key := GenerateResponseChunksKey(req.ProjectID, req.ID)

	data, err := s.DataStorageService.LoadData(ctx, dataStorage, key)
	if err != nil {
		log.Warn(ctx, "Failed to load request response chunks", log.Cause(err), log.Int("request_id", req.ID))

		return []objects.JSONRawMessage{}, nil
	}

	if len(data) == 0 {
		return []objects.JSONRawMessage{}, nil
	}

	var chunks []objects.JSONRawMessage
	if err := json.Unmarshal(data, &chunks); err != nil {
		log.Warn(ctx, "Failed to unmarshal request response chunks", log.Cause(err), log.Int("request_id", req.ID))
		return []objects.JSONRawMessage{}, nil
	}

	s.backfillRequestChunksAvailability(ctx, req, chunks)
	return chunks, nil
}

// LoadRequestExecutionRequestBody returns the execution request body, loading from external storage when necessary.
func (s *RequestService) LoadRequestExecutionRequestBody(ctx context.Context, exec *ent.RequestExecution) (objects.JSONRawMessage, error) {
	if exec == nil {
		return nil, fmt.Errorf("request execution is nil")
	}

	dataStorage, err := s.getDataStorage(ctx, exec.DataStorageID)
	if err != nil {
		log.Warn(ctx, "Failed to get data storage for execution request body", log.Cause(err), log.Int("execution_id", exec.ID))
		return xjson.EmptyJSONRawMessage, nil
	}

	if !s.shouldUseExternalStorage(ctx, dataStorage) {
		if exec.RequestBody == nil {
			return xjson.EmptyJSONRawMessage, nil
		}

		s.backfillExecutionRequestBodyAvailability(ctx, exec, exec.RequestBody)
		return exec.RequestBody, nil
	}

	key := GenerateExecutionRequestBodyKey(exec.ProjectID, exec.RequestID, exec.ID)

	data, err := s.DataStorageService.LoadData(ctx, dataStorage, key)
	if err != nil {
		return xjson.EmptyJSONRawMessage, nil
	}

	if json.Valid(data) {
		s.backfillExecutionRequestBodyAvailability(ctx, exec, data)
		return objects.JSONRawMessage(data), nil
	}

	return xjson.EmptyJSONRawMessage, nil
}

// LoadRequestExecutionResponseBody returns the execution response body, loading from external storage when necessary.
func (s *RequestService) LoadRequestExecutionResponseBody(ctx context.Context, exec *ent.RequestExecution) (objects.JSONRawMessage, error) {
	if exec == nil {
		return nil, fmt.Errorf("request execution is nil")
	}

	// Only load response body if execution is completed
	if exec.Status != requestexecution.StatusCompleted {
		return xjson.EmptyJSONRawMessage, nil
	}

	dataStorage, err := s.getDataStorage(ctx, exec.DataStorageID)
	if err != nil {
		log.Warn(ctx, "Failed to get data storage for execution response body", log.Cause(err), log.Int("execution_id", exec.ID))
		return xjson.EmptyJSONRawMessage, nil
	}

	if !s.shouldUseExternalStorage(ctx, dataStorage) {
		if exec.ResponseBody == nil {
			return xjson.EmptyJSONRawMessage, nil
		}

		s.backfillExecutionResponseBodyAvailability(ctx, exec, exec.ResponseBody)
		return exec.ResponseBody, nil
	}

	key := GenerateExecutionResponseBodyKey(exec.ProjectID, exec.RequestID, exec.ID)

	data, err := s.DataStorageService.LoadData(ctx, dataStorage, key)
	if err != nil {
		return xjson.EmptyJSONRawMessage, nil
	}

	if json.Valid(data) {
		s.backfillExecutionResponseBodyAvailability(ctx, exec, data)
		return objects.JSONRawMessage(data), nil
	}

	return xjson.EmptyJSONRawMessage, nil
}

// LoadRequestExecutionResponseChunks returns the execution response chunks, loading from external storage when necessary.
func (s *RequestService) LoadRequestExecutionResponseChunks(ctx context.Context, exec *ent.RequestExecution) ([]objects.JSONRawMessage, error) {
	if exec == nil {
		return nil, fmt.Errorf("request execution is nil")
	}

	// Live preview for active streaming executions
	if exec.Stream && exec.Status == requestexecution.StatusProcessing && s.LiveStreamRegistry != nil {
		chunks := s.LiveStreamRegistry.GetExecutionChunks(exec.ID)
		return chunks, nil
	}
	// Only load response body if execution is completed
	if !exec.Stream || exec.Status != requestexecution.StatusCompleted {
		return []objects.JSONRawMessage{}, nil
	}

	dataStorage, err := s.getDataStorage(ctx, exec.DataStorageID)
	if err != nil {
		log.Warn(ctx, "Failed to get data storage for execution response chunks", log.Cause(err), log.Int("execution_id", exec.ID))
		return []objects.JSONRawMessage{}, nil
	}

	if !s.shouldUseExternalStorage(ctx, dataStorage) {
		s.backfillExecutionChunksAvailability(ctx, exec, exec.ResponseChunks)
		return exec.ResponseChunks, nil
	}

	key := GenerateExecutionResponseChunksKey(exec.ProjectID, exec.RequestID, exec.ID)

	data, err := s.DataStorageService.LoadData(ctx, dataStorage, key)
	if err != nil {
		log.Warn(ctx, "Failed to load request execution response chunks", log.Cause(err), log.Int("execution_id", exec.ID))

		return []objects.JSONRawMessage{}, nil
	}

	if json.Valid(data) {
		var chunks []objects.JSONRawMessage
		if err := json.Unmarshal(data, &chunks); err != nil {
			log.Warn(ctx, "Failed to unmarshal request execution response chunks", log.Cause(err), log.Int("execution_id", exec.ID))
			return []objects.JSONRawMessage{}, nil
		}

		s.backfillExecutionChunksAvailability(ctx, exec, chunks)
		return chunks, nil
	}

	return []objects.JSONRawMessage{}, nil
}

func (s *RequestService) GetTraceFirstRequest(ctx context.Context, traceID int) (*ent.Request, error) {
	client := s.entFromContext(ctx)
	if client == nil {
		return nil, fmt.Errorf("ent client not found in context")
	}

	request, err := client.Request.Query().
		Where(request.TraceIDEQ(traceID), request.StatusEQ(request.StatusCompleted)).
		Order(ent.Asc(request.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get first request for trace: %w", err)
	}

	return request, nil
}

func (s *RequestService) GetTraceFirstSegment(ctx context.Context, traceID int) (*Segment, error) {
	request, err := s.GetTraceFirstRequest(ctx, traceID)
	if err != nil {
		return nil, err
	}

	if request == nil {
		return nil, nil
	}

	body, err := s.LoadRequestBody(ctx, request)
	if err != nil {
		return nil, err
	}

	request.RequestBody = body

	body, err = s.LoadResponseBody(ctx, request)
	if err != nil {
		return nil, err
	}

	request.ResponseBody = body

	return requestToSegment(ctx, request)
}

// GetPreviousChannelID retrieves the most recently selected channel ID from a trace.
// The cache is the source of truth; when it expires, trace affinity is reset.
// Returns 0 if no selected channel is cached.
func (s *RequestService) GetPreviousChannelID(ctx context.Context, traceID int) (int, error) {
	cacheKey := buildPreviousTraceChannelCacheKey(traceID)
	if channelID, err := s.previousChannelCache.Get(ctx, cacheKey); err == nil {
		return channelID, nil
	}

	return 0, nil
}

// GetPreviousChannelIDByThread retrieves the most recently selected channel
// from all traces associated with a thread. The cache is the source of truth;
// when it expires, thread affinity is reset. Returns 0 if none is cached.
func (s *RequestService) GetPreviousChannelIDByThread(ctx context.Context, threadID int) (int, error) {
	cacheKey := buildPreviousThreadChannelCacheKey(threadID)
	if channelID, err := s.previousChannelCache.Get(ctx, cacheKey); err == nil {
		return channelID, nil
	}

	return 0, nil
}

func (s *RequestService) cachePreviousChannelForRequest(ctx context.Context, req *ent.Request) {
	if req == nil || req.ChannelID == 0 || req.TraceID == 0 {
		return
	}

	s.setPreviousTraceChannelID(ctx, req.TraceID, req.ChannelID)

	threadID := 0
	if currentTrace, ok := contexts.GetTrace(ctx); ok && currentTrace.ID == req.TraceID {
		threadID = currentTrace.ThreadID
	}

	if threadID == 0 {
		currentTrace, err := s.entFromContext(ctx).Trace.Get(ctx, req.TraceID)
		if err != nil {
			log.Warn(ctx, "failed to get trace for previous channel cache", log.Cause(err), log.Int("trace_id", req.TraceID))
			return
		}
		threadID = currentTrace.ThreadID
	}

	if threadID != 0 {
		s.setPreviousThreadChannelID(ctx, threadID, req.ChannelID)
	}
}

func (s *RequestService) setPreviousTraceChannelID(ctx context.Context, traceID, channelID int) {
	s.setPreviousChannelCache(ctx, buildPreviousTraceChannelCacheKey(traceID), channelID, 30*time.Minute)
}

func (s *RequestService) setPreviousThreadChannelID(ctx context.Context, threadID, channelID int) {
	s.setPreviousChannelCache(ctx, buildPreviousThreadChannelCacheKey(threadID), channelID, 30*time.Minute)
}

func (s *RequestService) setPreviousChannelCache(ctx context.Context, cacheKey string, channelID int, expiration time.Duration) {
	_ = s.previousChannelCache.Set(ctx, cacheKey, channelID, store.WithExpiration(expiration))
}

func buildPreviousTraceChannelCacheKey(traceID int) string {
	return fmt.Sprintf("llm-proxy:routing:previous-channel:v1:trace:%d", traceID)
}

func buildPreviousThreadChannelCacheKey(threadID int) string {
	return fmt.Sprintf("llm-proxy:routing:previous-channel:v1:thread:%d", threadID)
}
