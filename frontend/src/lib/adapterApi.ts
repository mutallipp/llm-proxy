import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiRequest } from '@/lib/api-client';

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
