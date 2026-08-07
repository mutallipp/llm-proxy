import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiRequest } from '@/lib/api-client';
import { graphqlRequest } from '@/gql/graphql';

export type GatewayStatus = 'enabled' | 'disabled' | 'archived';
export type ApiFormat = string;

export interface AdapterBinding {
  id?: number;
  source_model_id: string;
  model_id: number;
  enabled: boolean;
  remark?: string;
}

export interface GatewayAdapter {
  id: number;
  name: string;
  display_name: string;
  inbound_api_format: ApiFormat;
  status: GatewayStatus | string;
  remark?: string;
  bindings: AdapterBinding[];
}

export interface AdapterUpdateInput {
  display_name: string;
  inbound_api_format: ApiFormat;
  status: GatewayStatus | string;
  remark?: string;
  bindings: Omit<AdapterBinding, 'id'>[];
}

export type StreamPolicy = 'unlimited' | 'require' | 'forbid';

export interface TargetCapabilities {
  supports_tools: boolean;
  supports_stream: boolean;
  /** 目标级流式策略：unlimited（跟随下游）| require（强制流式）| forbid（禁止流式）。缺省按 unlimited 处理。 */
  stream_policy?: StreamPolicy | string;
  supports_reasoning: boolean;
  /** 0 表示未声明上下文窗口。 */
  context_length?: number;
  /** 0 表示未声明最大输出 Token。 */
  max_output_tokens?: number;
  input_modalities: string[];
  output_modalities: string[];
}

export interface AdapterUpdateResponse {
  adapter: GatewayAdapter;
  snapshot_version: number;
  refreshed_at: string;
  diagnostics?: unknown;
}

export const INBOUND_API_FORMATS = [
  'openai/chat_completions',
  'openai/responses',
  'anthropic/messages',
] as const;

// 出站目标可使用的完整协议格式列表。
export const API_FORMATS = [
  'openai/chat_completions',
  'openai/responses',
  'openai/image_generation',
  'openai/image_edit',
  'openai/image_variation',
  'openai/embeddings',
  'openai/video',
  'openai/audio_speech',
  'openai/audio_transcriptions',
  'openai/audio_translations',
  'anthropic/messages',
  'gemini/contents',
  'gemini/embeddings',
  'aisdk/text',
  'aisdk/datastream',
  'jina/rerank',
  'jina/embeddings',
  'ollama/chat',
] as const;

export function useAdapters() {
  return useQuery({
    queryKey: ['gateway-adapters'],
    queryFn: () => apiRequest<{ adapters: GatewayAdapter[] }>('/admin/gateway/adapters', { requireAuth: true }),
  });
}


export function useUpsertAdapter() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: AdapterUpdateInput }) =>
      apiRequest<AdapterUpdateResponse>(`/admin/gateway/adapters/${encodeURIComponent(name)}`, {
        method: 'PUT',
        body: data,
        requireAuth: true,
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['gateway-adapters'] }),
  });
}


export function useRefreshGateway() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiRequest('/admin/gateway/refresh', { method: 'POST', requireAuth: true }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gateway-adapters'] });
      queryClient.invalidateQueries({ queryKey: ['gateway-runtime'] });
    },
  });
}

export function useGatewayRuntime() {
  return useQuery({
    queryKey: ['gateway-runtime'],
    queryFn: () => apiRequest<Record<string, unknown>>('/admin/gateway/runtime', { requireAuth: true }),
  });
}

// ===================== 重命名 Adapter =====================

export interface RenameAdapterResponse {
  adapter: GatewayAdapter;
  snapshot_version: number;
  refreshed_at: string;
}

export function useRenameAdapter() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, newName }: { name: string; newName: string }) =>
      apiRequest<RenameAdapterResponse>(`/admin/gateway/adapters/${encodeURIComponent(name)}/rename`, {
        method: 'POST',
        body: { new_name: newName },
        requireAuth: true,
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['gateway-adapters'] }),
  });
}

// ===================== 删除 Adapter =====================

export function useDeleteAdapter() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name }: { name: string }) =>
      apiRequest(`/admin/gateway/adapters/${encodeURIComponent(name)}`, {
        method: 'DELETE',
        requireAuth: true,
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['gateway-adapters'] }),
  });
}


// ===================== Adapter 绑定测试 =====================

export interface AdapterTestResult {
  latency: number;
  status: number;
  ok: boolean;
  requestID: string | null;
  error?: string;
}

const TEST_ADAPTER_MUTATION = `
  mutation TestAdapter($input: TestAdapterInput!) {
    testAdapter(input: $input) {
      latency
      success
      message
      error
      requestID
    }
  }
`;

export async function testAdapterBinding(params: {
  adapterName: string;
  sourceModelId: string;
}): Promise<AdapterTestResult> {
  const startedAt = Date.now();
  const data = await graphqlRequest<{
    testAdapter: {
      latency: number;
      success: boolean;
      message: string | null;
      error: string | null;
      requestID: string | null;
    };
  }>(TEST_ADAPTER_MUTATION, {
    input: { adapter: params.adapterName, modelID: params.sourceModelId },
  });
  const result = data.testAdapter;
  return {
    latency: result.latency || (Date.now() - startedAt) / 1000,
    status: 0,
    ok: result.success,
    requestID: result.requestID,
    error: result.error ?? (result.success ? undefined : result.message ?? undefined),
  };
}
