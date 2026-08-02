import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiRequest } from '@/lib/api-client';
import { getTokenFromStorage } from '@/stores/authStore';

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
  latency: number;      // 响应耗时（秒）
  summary: string;      // 简短内容摘要（成功时提取）
  status: number;       // HTTP 状态码，网络错误时为 0
  ok: boolean;          // 是否为 2xx
  rawResponse: string;  // 原始响应文本（JSON 字符串或纯文本）
  error?: string;       // 错误信息（非 2xx 或网络错误时填充）
}

/**
 * 直接调用 Adapter 路由发送最小非流式请求，用于验证链路连通性。
 * 使用当前管理员 JWT 以便追踪，不传消费端 API Key。
 * 非 2xx 不再抛出，而是返回含 ok:false 和 error 的结果，便于 UI 展示完整错误信息。
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

  // 捕获网络层错误（DNS 失败、连接拒绝等）
  let response: Response;
  try {
    response = await fetch(url, {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
    });
  } catch (networkError) {
    const latency = (Date.now() - startTime) / 1000;
    return {
      latency,
      summary: '',
      status: 0,
      ok: false,
      rawResponse: '',
      error: networkError instanceof Error ? networkError.message : '网络错误',
    };
  }

  const latency = (Date.now() - startTime) / 1000;
  // 先读取原始文本，保留完整响应内容供 UI 展示
  const rawResponse = await response.text().catch(() => '');

  if (!response.ok) {
    // 非 2xx：尝试从响应体中提取可读错误信息
    let errMsg = `HTTP ${response.status}`;
    try {
      const parsed = JSON.parse(rawResponse);
      if (parsed?.message) errMsg = parsed.message;
      else if (parsed?.error) errMsg = typeof parsed.error === 'string' ? parsed.error : (parsed.error?.message ?? errMsg);
    } catch {
      if (rawResponse) errMsg += `: ${rawResponse.slice(0, 200)}`;
    }
    return {
      latency,
      summary: '',
      status: response.status,
      ok: false,
      rawResponse,
      error: errMsg,
    };
  }

  // 2xx：解析 JSON 并提取简短摘要
  let data: Record<string, unknown> = {};
  try {
    data = JSON.parse(rawResponse);
  } catch {
    // 非 JSON 格式直接作为摘要返回
    return { latency, summary: rawResponse.slice(0, 120), status: response.status, ok: true, rawResponse };
  }

  let summary = '';
  if (inboundApiFormat === 'anthropic/messages') {
    summary = ((data?.content as Array<{ text?: string }>)?.[0]?.text ?? '').slice(0, 120);
  } else if (inboundApiFormat === 'openai/responses') {
    summary = ((data?.output as Array<{ content?: Array<{ text?: string }> }>)?.[0]?.content?.[0]?.text ?? '').slice(0, 120);
  } else {
    summary = ((data?.choices as Array<{ message?: { content?: string } }>)?.[0]?.message?.content ?? '').slice(0, 120);
  }

  return { latency, summary, status: response.status, ok: true, rawResponse };
}
