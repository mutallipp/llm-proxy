import type { Channel } from '@/features/channels/data/schema';
import { extractNumberID } from '@/lib/utils';

export const protocolPoolFormats = ['openai', 'anthropic'] as const;

export const protocolPoolEndpointApiFormatPrefixes: Record<string, readonly string[]> = {
  openai: ['openai/'],
  anthropic: ['anthropic/'],
};

type ChannelEndpointCapabilities = Pick<Channel, 'endpoints' | 'defaultEndpoints'>;

type ChannelIdentity = Pick<Channel, 'id'>;

export function parseChannelIdFromSelectValue(value: string): number | null {
  const isNumericId = /^[1-9]\d*$/.test(value);
  const isRelayGid = /^gid:\/\/llm-proxy\/Channel\/[1-9]\d*$/.test(value);
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
