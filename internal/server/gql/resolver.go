package gql

import (
	"errors"

	"github.com/99designs/gqlgen/graphql"

	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/server/backup"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
	"github.com/mutallipp/llm-proxy/internal/server/gc"
	"github.com/mutallipp/llm-proxy/internal/server/orchestrator"
	"github.com/mutallipp/llm-proxy/internal/server/scheduler"
	"github.com/mutallipp/llm-proxy/internal/server/video_storage"
	"github.com/mutallipp/llm-proxy/llm/httpclient"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

// ErrNotOwner is returned when a non-owner user attempts an owner-only operation.
var ErrNotOwner = errors.New("permission denied: owner access required")

// Resolver is the resolver root.
type Resolver struct {
	client                         *ent.Client
	authService                    *biz.AuthService
	apiKeyService                  *biz.APIKeyService
	userService                    *biz.UserService
	systemService                  *biz.SystemService
	channelService                 *biz.ChannelService
	requestService                 *biz.RequestService
	quotaService                   *biz.QuotaService
	projectService                 *biz.ProjectService
	dataStorageService             *biz.DataStorageService
	roleService                    *biz.RoleService
	traceService                   *biz.TraceService
	threadService                  *biz.ThreadService
	channelOverrideTemplateService *biz.ChannelOverrideTemplateService
	apiKeyProfileTemplateService   *biz.APIKeyProfileTemplateService
	modelService                   *biz.ModelService
	adapterService                 *biz.AdapterService
	backupService                  *backup.BackupService
	channelProbeService            *biz.ChannelProbeService
	promptService                  *biz.PromptService
	promptProtectionRuleService    *biz.PromptProtectionRuleService
	providerQuotaService           *biz.ProviderQuotaService
	scheduler                      *scheduler.Scheduler
	modelFetcher                   *biz.ModelFetcher
	defaultSelector                *orchestrator.DefaultSelector
	candidateSelectorDiagnostics   *orchestrator.CandidateSelectorDiagnostics
	channelLimiterManager          *orchestrator.ChannelLimiterManager
	TestChannelOrchestrator        *orchestrator.TestChannelOrchestrator
	gcWorker                       *gc.Worker
	videoWorker                    *video_storage.Worker
}

// NewSchema creates a graphql executable schema.
func NewSchema(
	client *ent.Client,
	authService *biz.AuthService,
	apiKeyService *biz.APIKeyService,
	userService *biz.UserService,
	systemService *biz.SystemService,
	channelService *biz.ChannelService,
	requestService *biz.RequestService,
	quotaService *biz.QuotaService,
	projectService *biz.ProjectService,
	dataStorageService *biz.DataStorageService,
	roleService *biz.RoleService,
	traceService *biz.TraceService,
	threadService *biz.ThreadService,
	usageLogService *biz.UsageLogService,
	channelOverrideTemplateService *biz.ChannelOverrideTemplateService,
	apiKeyProfileTemplateService *biz.APIKeyProfileTemplateService,
	modelService *biz.ModelService,
	adapterService *biz.AdapterService,
	backupService *backup.BackupService,
	channelProbeService *biz.ChannelProbeService,
	promptService *biz.PromptService,
	promptProtectionRuleService *biz.PromptProtectionRuleService,
	providerQuotaService *biz.ProviderQuotaService,
	scheduler *scheduler.Scheduler,
	defaultSelector *orchestrator.DefaultSelector,
	candidateSelectorDiagnostics *orchestrator.CandidateSelectorDiagnostics,
	channelLimiterManager *orchestrator.ChannelLimiterManager,
	httpClient *httpclient.HttpClient,
	gcWorker *gc.Worker,
	videoWorker *video_storage.Worker,
) graphql.ExecutableSchema {
	modelFetcher := biz.NewModelFetcher(httpClient, channelService)

	return NewExecutableSchema(Config{
		Resolvers: &Resolver{
			client:                         client,
			authService:                    authService,
			apiKeyService:                  apiKeyService,
			userService:                    userService,
			systemService:                  systemService,
			channelService:                 channelService,
			requestService:                 requestService,
			quotaService:                   quotaService,
			projectService:                 projectService,
			dataStorageService:             dataStorageService,
			roleService:                    roleService,
			traceService:                   traceService,
			threadService:                  threadService,
			channelOverrideTemplateService: channelOverrideTemplateService,
			apiKeyProfileTemplateService:   apiKeyProfileTemplateService,
			modelService:                   modelService,
			adapterService:                 adapterService,
			backupService:                  backupService,
			channelProbeService:            channelProbeService,
			promptService:                  promptService,
			promptProtectionRuleService:    promptProtectionRuleService,
			providerQuotaService:           providerQuotaService,
			scheduler:                      scheduler,
			modelFetcher:                   modelFetcher,
			defaultSelector:                defaultSelector,
			candidateSelectorDiagnostics:   candidateSelectorDiagnostics,
			channelLimiterManager:          channelLimiterManager,
			TestChannelOrchestrator:        orchestrator.NewTestChannelOrchestrator(channelService, requestService, systemService, usageLogService, promptProtectionRuleService, httpClient),
			gcWorker:                       gcWorker,
			videoWorker:                    videoWorker,
		},
	})
}
