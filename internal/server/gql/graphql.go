package gql

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"entgo.io/contrib/entgql"
	"entgo.io/ent/privacy"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.uber.org/fx"

	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/apikey"
	"github.com/mutallipp/llm-proxy/internal/ent/apikeyprofiletemplate"
	"github.com/mutallipp/llm-proxy/internal/ent/channel"
	"github.com/mutallipp/llm-proxy/internal/ent/channeloverridetemplate"
	"github.com/mutallipp/llm-proxy/internal/ent/channelprobe"
	"github.com/mutallipp/llm-proxy/internal/ent/datastorage"
	"github.com/mutallipp/llm-proxy/internal/ent/model"
	"github.com/mutallipp/llm-proxy/internal/ent/project"
	"github.com/mutallipp/llm-proxy/internal/ent/prompt"
	"github.com/mutallipp/llm-proxy/internal/ent/request"
	"github.com/mutallipp/llm-proxy/internal/ent/requestexecution"
	"github.com/mutallipp/llm-proxy/internal/ent/role"
	"github.com/mutallipp/llm-proxy/internal/ent/system"
	"github.com/mutallipp/llm-proxy/internal/ent/thread"
	"github.com/mutallipp/llm-proxy/internal/ent/trace"
	"github.com/mutallipp/llm-proxy/internal/ent/usagelog"
	"github.com/mutallipp/llm-proxy/internal/ent/user"
	"github.com/mutallipp/llm-proxy/internal/ent/userproject"
	"github.com/mutallipp/llm-proxy/internal/ent/userrole"
	"github.com/mutallipp/llm-proxy/internal/pkg/xerrors"
	"github.com/mutallipp/llm-proxy/internal/server/backup"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
	"github.com/mutallipp/llm-proxy/internal/server/gc"
	"github.com/mutallipp/llm-proxy/internal/server/orchestrator"
	"github.com/mutallipp/llm-proxy/internal/server/scheduler"
	"github.com/mutallipp/llm-proxy/internal/server/video_storage"
	"github.com/mutallipp/llm-proxy/llm/httpclient"
)

type Dependencies struct {
	fx.In

	Ent                            *ent.Client
	AuthService                    *biz.AuthService
	APIKeyService                  *biz.APIKeyService
	UserService                    *biz.UserService
	SystemService                  *biz.SystemService
	ChannelService                 *biz.ChannelService
	RequestService                 *biz.RequestService
	QuotaService                   *biz.QuotaService
	ProjectService                 *biz.ProjectService
	DataStorageService             *biz.DataStorageService
	RoleService                    *biz.RoleService
	TraceService                   *biz.TraceService
	ThreadService                  *biz.ThreadService
	UsageLogService                *biz.UsageLogService
	ChannelOverrideTemplateService *biz.ChannelOverrideTemplateService
	APIKeyProfileTemplateService   *biz.APIKeyProfileTemplateService
	ModelService                   *biz.ModelService
	AdapterService                 *biz.AdapterService
	BackupService                  *backup.BackupService
	ChannelProbeService            *biz.ChannelProbeService
	PromptService                  *biz.PromptService
	PromptProtectionRuleService    *biz.PromptProtectionRuleService
	ProviderQuotaService           *biz.ProviderQuotaService
	Scheduler                      *scheduler.Scheduler
	DefaultSelector                *orchestrator.DefaultSelector
	CandidateSelectorDiagnostics   *orchestrator.CandidateSelectorDiagnostics
	ChannelLimiterManager          *orchestrator.ChannelLimiterManager
	HttpClient                     *httpclient.HttpClient
	GCWorker                       *gc.Worker
	VideoWorker                    *video_storage.Worker
}

type GraphqlHandler struct {
	Graphql    http.Handler
	Playground http.Handler
}

func NewGraphqlHandlers(deps Dependencies) *GraphqlHandler {
	gqlSrv := handler.New(
		NewSchema(
			deps.Ent,
			deps.AuthService,
			deps.APIKeyService,
			deps.UserService,
			deps.SystemService,
			deps.ChannelService,
			deps.RequestService,
			deps.QuotaService,
			deps.ProjectService,
			deps.DataStorageService,
			deps.RoleService,
			deps.TraceService,
			deps.ThreadService,
			deps.UsageLogService,
			deps.ChannelOverrideTemplateService,
			deps.APIKeyProfileTemplateService,
			deps.ModelService,
			deps.AdapterService,
			deps.BackupService,
			deps.ChannelProbeService,
			deps.PromptService,
			deps.PromptProtectionRuleService,
			deps.ProviderQuotaService,
			deps.Scheduler,
			deps.DefaultSelector,
			deps.CandidateSelectorDiagnostics,
			deps.ChannelLimiterManager,
			deps.HttpClient,
			deps.GCWorker,
			deps.VideoWorker,
		),
	)

	gqlSrv.AddTransport(transport.Options{})
	gqlSrv.AddTransport(transport.GET{})
	gqlSrv.AddTransport(transport.POST{})
	gqlSrv.AddTransport(transport.MultipartForm{})

	gqlSrv.SetQueryCache(lru.New[*ast.QueryDocument](1024))

	gqlSrv.Use(extension.Introspection{})
	gqlSrv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](1024),
	})
	gqlSrv.Use(&loggingTracer{})
	skipTestChannelTransaction := entgql.SkipOperations("TestChannel", "TestChannelAPIKeys", "TestModel", "TestAdapter")
	skipBulkImportTransaction := entgql.SkipIfHasFields("bulkImportChannels")
	gqlSrv.Use(entgql.Transactioner{
		TxOpener: deps.Ent,
		// 三类测试入口会执行长时间的 Provider 请求，不应持有 GraphQL 数据库事务；
		// BulkImportChannels 仍按行管理事务以保留部分成功语义。
		SkipTxFunc: func(op *ast.OperationDefinition) bool {
			return skipTestChannelTransaction(op) || skipBulkImportTransaction(op)
		},
	})

	// Set error presenter to handle CodedError and add extensions.code
	gqlSrv.SetErrorPresenter(func(ctx context.Context, err error) *gqlerror.Error {
		// Check if it's a CodedError
		var codedErr *xerrors.CodedError
		if errors.As(err, &codedErr) {
			return &gqlerror.Error{
				Message: codedErr.Message,
				Extensions: map[string]any{
					"code":     codedErr.Code,
					"resource": codedErr.Extensions["resource"],
					"field":    codedErr.Extensions["field"],
					"value":    codedErr.Extensions["value"],
				},
			}
		}
		// Convert ent privacy deny errors to FORBIDDEN
		if errors.Is(err, privacy.Deny) {
			return &gqlerror.Error{
				Message: "permission denied",
				Extensions: map[string]any{
					"code": xerrors.ErrCodeForbidden,
				},
			}
		}
		// Return default error presentation
		return graphql.DefaultErrorPresenter(ctx, err)
	})

	return &GraphqlHandler{
		Graphql:    gqlSrv,
		Playground: playground.Handler("llm-proxy", "/admin/graphql"),
	}
}

var guidTypeToNodeType = map[string]string{
	ent.TypeUser:                    user.Table,
	ent.TypeAPIKey:                  apikey.Table,
	ent.TypeAPIKeyProfileTemplate:   apikeyprofiletemplate.Table,
	ent.TypeModel:                   model.Table,
	ent.TypeChannel:                 channel.Table,
	ent.TypeChannelProbe:            channelprobe.Table,
	ent.TypeChannelOverrideTemplate: channeloverridetemplate.Table,
	ent.TypeRequest:                 request.Table,
	ent.TypeRequestExecution:        requestexecution.Table,
	ent.TypeRole:                    role.Table,
	ent.TypeSystem:                  system.Table,
	ent.TypeUsageLog:                usagelog.Table,
	ent.TypeProject:                 project.Table,
	ent.TypeUserProject:             userproject.Table,
	ent.TypeUserRole:                userrole.Table,
	ent.TypeThread:                  thread.Table,
	ent.TypeTrace:                   trace.Table,
	ent.TypeDataStorage:             datastorage.Table,
	ent.TypePrompt:                  prompt.Table,
}

func getNilableChannel(ctx context.Context, client *ent.Client, channelID int) (*ent.Channel, error) {
	if channelID == 0 {
		return nil, nil
	}

	ch, err := client.Channel.Query().Where(channel.ID(channelID)).First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}

		if errors.Is(err, privacy.Deny) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to load channel: %w", err)
	}

	return ch, nil
}

func getNilableUser(ctx context.Context, client *ent.Client, userID int) (*ent.User, error) {
	if userID == 0 {
		return nil, nil
	}

	u, err := client.User.Query().Where(user.ID(userID)).First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}

		if errors.Is(err, privacy.Deny) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to load user: %w", err)
	}

	return u, nil
}
