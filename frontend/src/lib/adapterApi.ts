import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiRequest } from '@/lib/api-client';
import { getTokenFromStorage } from '@/stores/authStore';

export type GatewayStatus = 'enabled' | 'disabled' | 'archived';
export type ApiFormat = string;

export interface AdapterBinding {
  id?: number;
  source_model_id: string;
  model_group_id: number;
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

export interface TargetCapabilities {
  supports_tools: boolean;
  supports_stream: boolean;
  input_modalities: string[];
  output_modalities: string[];
}

export interface ModelGroupTarget {
  id?: number;
  channel_id: number;
  target_model_id: string;
  outbound_api_format: ApiFormat;
  priority: number;
  enabled: boolean;
  remark?: string;
  capabilities: TargetCapabilities;
}

export interface ModelGroupProtocol {
  id?: number;
  inbound_api_format: ApiFormat;
  enabled: boolean;
  remark?: string;
  targets: ModelGroupTarget[];
}

export type ModelGroupTargetInput = Omit<ModelGroupTarget, 'id'>;
export type ModelGroupProtocolInput = Omit<ModelGroupProtocol, 'id' | 'targets'> & {
  targets: ModelGroupTargetInput[];
};

export interface ModelGroup {
  id: number;
  name: string;
  display_name: string;
  status: GatewayStatus | string;
  selection_strategy: string;
  remark?: string;
  protocols: ModelGroupProtocol[];
}

export interface ModelGroupUpdateInput {
  display_name: string;
  status: GatewayStatus | string;
  selection_strategy: string;
  remark?: string;
  protocols: ModelGroupProtocolInput[];
}

export interface AdapterUpdateResponse {
  adapter: GatewayAdapter;
  snapshot_version: number;
  refreshed_at: string;
  diagnostics?: unknown;
}

export interface ModelGroupUpdateResponse {
  model_group: ModelGroup;
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

export function useModelGroups() {
  return useQuery({
    queryKey: ['gateway-model-groups'],
    queryFn: () => apiRequest<{ model_groups: ModelGroup[] }>('/admin/gateway/model-groups', { requireAuth: true }),
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

export function useUpsertModelGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: ModelGroupUpdateInput }) =>
      apiRequest<ModelGroupUpdateResponse>(`/admin/gateway/model-groups/${encodeURIComponent(name)}`, {
        method: 'PUT',
        body: data,
        requireAuth: true,
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['gateway-model-groups'] }),
  });
}

export function useRefreshGateway() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiRequest('/admin/gateway/refresh', { method: 'POST', requireAuth: true }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gateway-adapters'] });
      queryClient.invalidateQueries({ queryKey: ['gateway-model-groups'] });
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

// ===================== 删除 ModelGroup =====================

export function useDeleteModelGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name }: { name: string }) =>
      apiRequest(`/admin/gateway/model-groups/${encodeURIComponent(name)}`, {
        method: 'DELETE',
        requireAuth: true,
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['gateway-model-groups'] }),
  });
}

// ===================== Adapter 绑定测试 =====================

export interface AdapterTestResult {
  latency: number;
  summary: string;
}

/**
 * 直接调用 Adapter 路由发送最小非流式请求，用于验证链路连通性。
 * 使用当前管理员 JWT 以便追踪，不传消费端 API Key。
 */
export async function testAdapterBinding(params: {
  adapterName: string;
  inboundApiFormat: string;
  sourceModelId: string;
}): Promise<AdapterTestResult> {
  const { adapterName, inboundApiFormat, sourceModelId } = params;
  const startTime = Date.now();

  let url: string;
  let body: Record<string, unknown>;

  if (inboundApiFormat === 'anthropic/messages') {
    url = `/${encodeURIComponent(adapterName)}/v1/messages`;
    body = {
      model: sourceModelId,
      max_tokens: 16,
      messages: [{ role: 'user', content: 'Reply with OK.' }],
      stream: false,
    };
  } else if (inboundApiFormat === 'openai/responses') {
    url = `/${encodeURIComponent(adapterName)}/v1/responses`;
    body = {
      model: sourceModelId,
      input: 'Reply with OK.',
      max_output_tokens: 16,
      stream: false,
    };
  } else {
    // openai/chat_completions（默认）
    url = `/${encodeURIComponent(adapterName)}/v1/chat/completions`;
    body = {
      model: sourceModelId,
      max_tokens: 16,
      messages: [{ role: 'user', content: 'Reply with OK.' }],
      stream: false,
    };
  }

  const token = getTokenFromStorage();
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  // 使用管理员 JWT 进行请求追踪，适配器中间件会在转发前移除此凭证
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const response = await fetch(url, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  });

  const latency = (Date.now() - startTime) / 1000;

  if (!response.ok) {
    const errorText = await response.text().catch(() => '');
    let errMsg = `HTTP ${response.status}`;
    try {
      const parsed = JSON.parse(errorText);
      if (parsed?.message) errMsg = parsed.message;
      else if (parsed?.error) errMsg = typeof parsed.error === 'string' ? parsed.error : parsed.error?.message ?? errMsg;
    } catch {
      if (errorText) errMsg += `: ${errorText.slice(0, 200)}`;
    }
    throw new Error(errMsg);
  }

  const data = await response.json().catch(() => ({}));

  // 提取简短响应摘要
  let summary = '';
  if (inboundApiFormat === 'anthropic/messages') {
    summary = (data?.content?.[0]?.text ?? '').slice(0, 120);
  } else if (inboundApiFormat === 'openai/responses') {
    summary = (data?.output?.[0]?.content?.[0]?.text ?? '').slice(0, 120);
  } else {
    summary = (data?.choices?.[0]?.message?.content ?? '').slice(0, 120);
  }

  return { latency, summary };
}
