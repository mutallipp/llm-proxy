import type { Channel } from '@/features/channels/data/schema';

export const protocolPoolFormats = ['openai', 'anthropic'] as const;

export const protocolPoolEndpointApiFormatPrefixes: Record<string, readonly string[]> = {
  openai: ['openai/'],
  anthropic: ['anthropic/'],
};

type ChannelEndpointCapabilities = Pick<Channel, 'endpoints' | 'defaultEndpoints'>;

export function channelSupportsProtocolPool(channel: ChannelEndpointCapabilities, protocol: string): boolean {
  const apiFormatPrefixes = protocolPoolEndpointApiFormatPrefixes[protocol];
  if (!apiFormatPrefixes) return false;

  return [...(channel.endpoints ?? []), ...(channel.defaultEndpoints ?? [])].some(({ apiFormat }) =>
    apiFormatPrefixes.some((prefix) => apiFormat.startsWith(prefix))
  );
}
