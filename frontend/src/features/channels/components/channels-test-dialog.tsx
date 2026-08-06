'use client';

import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { IconLoader2, IconPlayerPlay } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useTestChannel } from '../data/channels';
import { Channel } from '../data/schema';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channel: Channel;
}

const protocolLabelKeys: Record<string, string> = {
  openai: 'channels.dialogs.test.protocolName.openai',
  openai_responses: 'channels.dialogs.test.protocolName.openaiResponses',
  anthropic: 'channels.dialogs.test.protocolName.anthropic',
};

export function ChannelsTestDialog({ open, onOpenChange, channel }: Props) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const testChannel = useTestChannel({ silent: true });
  const protocols = useMemo(() => channel.protocolCapabilities?.declaredProtocols ?? [], [channel.protocolCapabilities]);
  const [protocol, setProtocol] = useState('');
  const [modelID, setModelID] = useState('');
  const models = useMemo(() => {
    const capabilityModels = channel.protocolCapabilities?.models
      .filter((model) => !protocol || model.protocols.includes(protocol))
      .map((model) => model.modelId) ?? [];
    return capabilityModels.length > 0 ? capabilityModels : channel.supportedModels;
  }, [channel.protocolCapabilities, channel.supportedModels, protocol]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    const nextProtocol = protocols[0] ?? '';
    setProtocol(nextProtocol);
    const protocolModels = channel.protocolCapabilities?.models.filter((model) => model.protocols.includes(nextProtocol)).map((model) => model.modelId) ?? [];
    setModelID(channel.defaultTestModel && protocolModels.includes(channel.defaultTestModel) ? channel.defaultTestModel : protocolModels[0] || models[0] || '');
    setError(null);
  }, [open, channel.defaultTestModel, models, protocols]);

  const runTest = async () => {
    if (!protocol || !modelID) return;
    setError(null);
    try {
      const result = await testChannel.mutateAsync({ channelID: channel.id, modelID, protocol });
      if (result.requestID) {
        onOpenChange(false);
        await navigate({ to: '/requests/$requestId', params: { requestId: result.requestID } });
        return;
      }
      if (!result.success) setError(result.error || result.message || t('channels.dialogs.test.failed'));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('channels.dialogs.test.failed'));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='w-[95vw] max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('channels.dialogs.test.title')}</DialogTitle>
          <DialogDescription>{t('channels.dialogs.test.fixedTemplateDescription', { name: channel.name })}</DialogDescription>
        </DialogHeader>
        <div className='space-y-4'>
          <div className='rounded-md border bg-muted/30 p-3 text-sm'>
            <Badge variant='secondary'>{t('channels.dialogs.test.fixedTemplate')}</Badge>
            <p className='text-muted-foreground mt-2'>{t('channels.dialogs.test.fixedTemplateText')}</p>
          </div>
          <div className='space-y-2'>
            <Label>{t('channels.dialogs.test.protocol')}</Label>
            <Select
              value={protocol}
              onValueChange={(value) => {
                setProtocol(value);
                const nextModel = channel.protocolCapabilities?.models.find((item) => item.protocols.includes(value))?.modelId;
                if (nextModel) setModelID(nextModel);
              }}
              disabled={protocols.length === 0}
            >
              <SelectTrigger><SelectValue placeholder={t('channels.dialogs.test.selectProtocol')} /></SelectTrigger>
              <SelectContent>
                {protocols.map((item) => <SelectItem key={item} value={item}>{protocolLabelKeys[item] ? t(protocolLabelKeys[item]) : item}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
          <div className='space-y-2'>
            <Label>{t('channels.dialogs.test.model')}</Label>
            <Select value={modelID} onValueChange={setModelID} disabled={models.length === 0}>
              <SelectTrigger><SelectValue placeholder={t('channels.dialogs.test.selectModel')} /></SelectTrigger>
              <SelectContent>
                {models.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
          {protocols.length === 0 && <p className='text-destructive text-sm'>{t('channels.dialogs.test.noProtocol')}</p>}
          {models.length === 0 && <p className='text-destructive text-sm'>{t('channels.dialogs.test.noModel')}</p>}
          {error && <p className='text-destructive rounded-md border border-destructive/30 p-3 text-sm'>{error}</p>}
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>{t('common.buttons.cancel')}</Button>
          <Button onClick={runTest} disabled={testChannel.isPending || !protocol || !modelID}>
            {testChannel.isPending ? <IconLoader2 className='mr-2 h-4 w-4 animate-spin' /> : <IconPlayerPlay className='mr-2 h-4 w-4' />}
            {testChannel.isPending ? t('channels.dialogs.test.testing') : t('channels.actions.test')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
