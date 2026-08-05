import { useCallback, useEffect, useMemo, useState } from 'react';
import { AlertCircle, Check, RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  useBulkEnableDerivedAssociations,
  useFetchModels,
  useSaveChannelCapabilities,
} from '../data/channels';
import {
  saveChannelCapabilitiesInputSchema,
  type Channel,
  type SaveChannelCapabilitiesInput,
} from '../data/schema';
import {
  channelSupportsProtocolPool,
  protocolPoolFormats,
  protocolPoolLabelKeys,
} from '@/features/models/data/protocol-pools';

type ProtocolPoolFormat = (typeof protocolPoolFormats)[number];

type SaveSummary = {
  addedCount: number;
  unmatchedModels: string[];
  autoDisabledCount: number;
  manualNotices: string[];
};

interface Props {
  channel: Channel;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function uniqueModels(channel: Channel, fetchedModels: string[]): string[] {
  return [...new Set([...(channel.supportedModels ?? []), ...(channel.manualModels ?? []), ...fetchedModels])].filter(Boolean);
}

export function ChannelsCapabilityDialog({ channel, open, onOpenChange }: Props) {
  const { t } = useTranslation();
  const saveCapabilities = useSaveChannelCapabilities();
  const bulkEnable = useBulkEnableDerivedAssociations();
  const fetchModels = useFetchModels();
  const [declaredProtocols, setDeclaredProtocols] = useState<ProtocolPoolFormat[]>([]);
  const [modelProtocols, setModelProtocols] = useState<Record<string, ProtocolPoolFormat[]>>({});
  const [fetchedModels, setFetchedModels] = useState<string[]>([]);
  const [error, setError] = useState('');
  const [summary, setSummary] = useState<SaveSummary | null>(null);
  const [bulkResult, setBulkResult] = useState<{ success: number; failed: string[] } | null>(null);

  const models = useMemo(() => uniqueModels(channel, fetchedModels), [channel, fetchedModels]);

  useEffect(() => {
    if (!open) return;
    const declared = (channel.protocolCapabilities?.declaredProtocols ?? []).filter(
      (protocol): protocol is ProtocolPoolFormat => protocolPoolFormats.includes(protocol as ProtocolPoolFormat)
    );
    const savedModels = new Map((channel.protocolCapabilities?.models ?? []).map((item) => [item.modelId, item.protocols]));
    setDeclaredProtocols(declared);
    setModelProtocols(
      Object.fromEntries(
        uniqueModels(channel, []).map((modelId) => [
          modelId,
          (savedModels.get(modelId) ?? declared).filter((protocol): protocol is ProtocolPoolFormat =>
            protocolPoolFormats.includes(protocol as ProtocolPoolFormat)
          ),
        ])
      )
    );
    setFetchedModels([]);
    setError('');
    setSummary(null);
    setBulkResult(null);
  }, [channel, open]);

  useEffect(() => {
    if (!fetchModels.data?.models.length) return;
    const fetched = fetchModels.data.models.map((model) => model.id).filter(Boolean);
    setFetchedModels(fetched);
    setModelProtocols((previous) => {
      const next = { ...previous };
      fetched.forEach((modelId) => {
        next[modelId] ??= [...declaredProtocols];
      });
      return next;
    });
  }, [declaredProtocols, fetchModels.data]);

  const handleProtocolChange = useCallback(
    (protocol: ProtocolPoolFormat, checked: boolean) => {
      if (checked && !channelSupportsProtocolPool(channel, protocol)) {
        setError(t('channels.capability.errors.endpointUnsupported', { protocol }));
        return;
      }
      setError('');
      setDeclaredProtocols((previous) => {
        const next = checked ? [...previous, protocol] : previous.filter((item) => item !== protocol);
        setModelProtocols((current) =>
          Object.fromEntries(
            Object.entries(current).map(([modelId, protocols]) => [
              modelId,
              checked ? [...new Set([...protocols, protocol])] : protocols.filter((item) => item !== protocol),
            ])
          )
        );
        return next;
      });
    },
    [channel, t]
  );

  const handleModelProtocolChange = (modelId: string, protocol: ProtocolPoolFormat, checked: boolean) => {
    setModelProtocols((previous) => ({
      ...previous,
      [modelId]: checked
        ? [...new Set([...(previous[modelId] ?? []), protocol])]
        : (previous[modelId] ?? []).filter((item) => item !== protocol),
    }));
  };

  const handleFetchModels = async () => {
    try {
      await fetchModels.mutateAsync({
        channelType: channel.type,
        baseURL: channel.baseURL,
        apiKey: channel.credentials?.apiKey ?? channel.credentials?.apiKeys?.[0],
        channelID: channel.id,
      });
    } catch {
      // 错误已由 hook 统一处理。
    }
  };

  const handleSave = async () => {
    setError('');
    const input: SaveChannelCapabilitiesInput = {
      channelID: channel.id,
      declaredProtocols,
      models: models.map((modelId) => ({ modelId, protocols: modelProtocols[modelId] ?? declaredProtocols })),
    };
    const parsed = saveChannelCapabilitiesInputSchema.safeParse(input);
    if (!parsed.success) {
      setError(t('channels.capability.errors.invalidInput'));
      return;
    }
    try {
      const result = await saveCapabilities.mutateAsync(parsed.data);
      setSummary({
        addedCount: result.addedCount,
        unmatchedModels: result.unmatchedModels,
        autoDisabledCount: result.revoked.autoDisabledCount,
        manualNotices: result.revoked.manualNotices,
      });
    } catch {
      // 错误已由 hook 统一处理。
    }
  };

  const handleBulkEnable = async () => {
    try {
      const result = await bulkEnable.mutateAsync({ channelID: channel.id });
      const failed = result.results.filter((item) => !item.success).map((item) => item.reason || t('channels.capability.bulkEnable.unknownFailure'));
      setBulkResult({
        success: result.results.length - failed.length,
        failed,
      });
    } catch {
      // 错误已由 hook 统一处理。
    }
  };

  const handleClose = (nextOpen: boolean) => {
    if (!nextOpen && !saveCapabilities.isPending && !bulkEnable.isPending) onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent data-testid='capability-dialog' className='grid max-h-[90vh] min-h-0 grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-3xl'>
        <DialogHeader className='min-h-0 text-left'>
          <DialogTitle>{t('channels.capability.title')}</DialogTitle>
          <DialogDescription>{t('channels.capability.description')}</DialogDescription>
        </DialogHeader>

        <ScrollArea className='min-h-0 pr-4'>
          <div className='space-y-5 pb-1'>
            <section className='space-y-3 rounded-md border p-3'>
              <div>
                <h3 className='font-medium'>{t('channels.capability.protocols.title')}</h3>
                <p className='text-muted-foreground text-sm'>{t('channels.capability.protocols.description')}</p>
              </div>
              <div className='flex flex-wrap gap-2'>
                {protocolPoolFormats.map((protocol) => {
                  const supported = channelSupportsProtocolPool(channel, protocol);
                  return (
                    <label key={protocol} className='flex items-center gap-2 rounded-md border px-3 py-2 text-sm'>
                      <Checkbox
                        data-testid='capability-protocol-checkbox'
                        data-protocol={protocol}
                        checked={declaredProtocols.includes(protocol)}
                        onCheckedChange={(checked) => handleProtocolChange(protocol, checked === true)}
                        disabled={!supported}
                      />
                      <span>{t(protocolPoolLabelKeys[protocol])}</span>
                      {!supported && <Badge variant='outline'>{t('channels.capability.protocols.endpointMissing')}</Badge>}
                    </label>
                  );
                })}
              </div>
            </section>

            <section className='space-y-3 rounded-md border p-3'>
              <div className='flex items-center justify-between gap-2'>
                <div>
                  <h3 className='font-medium'>{t('channels.capability.models.title')}</h3>
                  <p className='text-muted-foreground text-sm'>{t('channels.capability.models.description')}</p>
                </div>
                <Button type='button' variant='outline' size='sm' onClick={handleFetchModels} disabled={fetchModels.isPending}>
                  <RefreshCw className='mr-2 h-4 w-4' />
                  {t('channels.capability.models.fetch')}
                </Button>
              </div>
              {models.length === 0 ? (
                <p className='text-muted-foreground rounded-md border border-dashed p-4 text-center text-sm'>
                  {t('channels.capability.models.empty')}
                </p>
              ) : (
                <div className='space-y-2'>
                  {models.map((modelId) => (
                    <div key={modelId} className='rounded-md border p-3'>
                      <div className='mb-2 flex items-center justify-between gap-2'>
                        <span className='truncate font-mono text-sm'>{modelId}</span>
                        <Badge variant='secondary'>{t('channels.capability.models.inherited')}</Badge>
                      </div>
                      <div className='flex flex-wrap gap-3'>
                        {protocolPoolFormats.map((protocol) => (
                          <label key={protocol} className='flex items-center gap-2 text-sm'>
                            <Checkbox
                              data-testid='capability-model-protocol-checkbox'
                              data-model={modelId}
                              data-protocol={protocol}
                              checked={(modelProtocols[modelId] ?? []).includes(protocol)}
                              onCheckedChange={(checked) => handleModelProtocolChange(modelId, protocol, checked === true)}
                              disabled={!declaredProtocols.includes(protocol)}
                            />
                            {protocol}
                          </label>
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </section>

            {error && (
              <div data-testid='capability-error' className='text-destructive bg-destructive/10 flex items-center gap-2 rounded-md px-3 py-2 text-sm'>
                <AlertCircle className='h-4 w-4 shrink-0' />
                {error}
              </div>
            )}

            {summary && (
              <section data-testid='capability-summary' className='space-y-2 rounded-md border border-green-500/30 bg-green-500/5 p-3 text-sm'>
                <p className='flex items-center gap-2 font-medium'><Check className='h-4 w-4 text-green-600' />{t('channels.capability.saved', { count: summary.addedCount })}</p>
                {summary.unmatchedModels.length > 0 && <p>{t('channels.capability.unmatched', { models: summary.unmatchedModels.join(', ') })}</p>}
                {summary.autoDisabledCount > 0 && <p>{t('channels.capability.revoked.autoDisabled', { count: summary.autoDisabledCount })}</p>}
                {summary.manualNotices.map((notice) => <p key={notice}>{t('channels.capability.revoked.manualNotice', { notice })}</p>)}
                {summary.addedCount > 0 && (
                  <Button data-testid='capability-bulk-enable' type='button' size='sm' variant='outline' onClick={handleBulkEnable} disabled={bulkEnable.isPending}>
                    {t('channels.capability.bulkEnable.button')}
                  </Button>
                )}
                {bulkResult && <p>{t('channels.capability.bulkEnable.result', { success: bulkResult.success, failed: bulkResult.failed.length })}</p>}
                {bulkResult?.failed.map((reason) => <p key={reason} className='text-destructive'>{reason}</p>)}
              </section>
            )}
          </div>
        </ScrollArea>

        <DialogFooter className='min-h-0 border-t bg-background pt-4'>
          <Button variant='outline' onClick={() => onOpenChange(false)}>{t('common.buttons.cancel')}</Button>
          <Button data-testid='capability-save' onClick={handleSave} disabled={saveCapabilities.isPending}>{t('common.buttons.save')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
