import type { Channel } from '@/features/channels/data/schema';
import { extractNumberID } from '@/lib/utils';

export const protocolPoolFormats = ['openai', 'openai_responses', 'anthropic'] as const;

export const protocolPoolLabelKeys: Record<(typeof protocolPoolFormats)[number], string> = {
  openai: 'channels.capability.protocols.openai',
  openai_responses: 'channels.capability.protocols.openaiResponses',
  anthropic: 'channels.capability.protocols.anthropic',
};

export const protocolPoolEndpointApiFormatPrefixes: Record<string, readonly string[]> = {
  openai: ['openai/chat_completions'],
  openai_responses: ['openai/responses'],
  anthropic: ['anthropic/'],
};

type ChannelEndpointCapabilities = Pick<Channel, 'endpoints' | 'defaultEndpoints'>;

type ChannelIdentity = Pick<Channel, 'id'>;

export function parseChannelIdFromSelectValue(value: string): number | null {
  const isNumericId = /^[1-9]\d*$/.test(value);
  const isRelayGid = /^gid:\/\/axonhub\/Channel\/[1-9]\d*$/.test(value);
  if (!isNumericId && !isRelayGid) return null;

  const channelId = Number(extractNumberID(value));
  return Number.isSafeInteger(channelId) ? channelId : null;
}

export function channelIdToSelectValue(
  channelId: number | null | undefined,
  channels: readonly ChannelIdentity[]
): string {
  if (!Number.isSafeInteger(channelId) || channelId <= 0) return '';
  return channels.find((channel) => parseChannelIdFromSelectValue(channel.id) === channelId)?.id ?? '';
}

export function channelSupportsProtocolPool(channel: ChannelEndpointCapabilities, protocol: string): boolean {
  const apiFormatPrefixes = protocolPoolEndpointApiFormatPrefixes[protocol];
  if (!apiFormatPrefixes) return false;

  return [...(channel.endpoints ?? []), ...(channel.defaultEndpoints ?? [])].some(({ apiFormat }) =>
    apiFormatPrefixes.some((prefix) => apiFormat.startsWith(prefix))
  );
}
