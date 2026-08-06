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
import { TestOriginHistoryDrawer } from '@/features/requests/components/test-origin-history-drawer';
import { useTestModel, useTestModelTargets } from '../data/models';
import { Model } from '../data/schema';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  model: Model;
}

const protocolLabelKeys: Record<string, string> = {
  openai: 'models.test.protocolName.openai',
  openai_responses: 'models.test.protocolName.openaiResponses',
  anthropic: 'models.test.protocolName.anthropic',
};

export function ModelsTestDialog({ open, onOpenChange, model }: Props) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const testModel = useTestModel({ silent: true });
  const protocols = useMemo(() => model.settings.protocolPools.map((pool) => pool.format), [model.settings.protocolPools]);
  const [protocol, setProtocol] = useState('');
  const [channelID, setChannelID] = useState('');
  const [physicalModelID, setPhysicalModelID] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [historyOpen, setHistoryOpen] = useState(false);

  const { data: targets = [], isLoading: targetsLoading, isError: targetsError } = useTestModelTargets(model.modelID, protocol, {
    enabled: open,
  });

  const channels = useMemo(() => {
    const seen = new Set<string>();
    return targets.filter((target) => {
      if (seen.has(target.channelID)) return false;
      seen.add(target.channelID);
      return true;
    });
  }, [targets]);

  const physicalModels = useMemo(
    () => targets.filter((target) => target.channelID === channelID),
    [channelID, targets]
  );

  useEffect(() => {
    if (open) {
      setProtocol(protocols[0] ?? '');
      setChannelID('');
      setPhysicalModelID('');
      setError(null);
    }
  }, [open, protocols]);

  useEffect(() => {
    setChannelID('');
    setPhysicalModelID('');
    setError(null);
  }, [protocol]);

  const runTest = async () => {
    if (!protocol || !channelID || !physicalModelID) return;
    setError(null);
    try {
      const result = await testModel.mutateAsync({ modelID: model.modelID, protocol, channelID, physicalModelID });
      if (result.requestID) {
        onOpenChange(false);
        await navigate({ to: '/requests/$requestId', params: { requestId: result.requestID } });
        return;
      }
      if (!result.success) setError(result.error || result.message || t('models.test.failed'));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('models.test.failed'));
    }
  };

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className='w-[95vw] max-w-lg'>
          <DialogHeader>
            <DialogTitle>{t('models.test.title')}</DialogTitle>
            <DialogDescription>{t('models.test.description', { name: model.name })}</DialogDescription>
          </DialogHeader>
          <div className='space-y-4'>
            <div className='rounded-md border bg-muted/30 p-3 text-sm'>
              <Badge variant='secondary'>{t('models.test.fixedTemplate')}</Badge>
              <p className='text-muted-foreground mt-2'>{t('models.test.fixedTemplateText')}</p>
            </div>
            <div className='space-y-2'>
              <Label>{t('models.test.protocol')}</Label>
              <Select value={protocol} onValueChange={setProtocol} disabled={protocols.length === 0}>
                <SelectTrigger><SelectValue placeholder={t('models.test.selectProtocol')} /></SelectTrigger>
                <SelectContent>
                  {protocols.map((item) => (
                    <SelectItem key={item} value={item}>{protocolLabelKeys[item] ? t(protocolLabelKeys[item]) : item}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {protocol && (
              <div className='space-y-2'>
                <Label>{t('models.test.channel')}</Label>
                <Select value={channelID} onValueChange={(value) => { setChannelID(value); setPhysicalModelID(''); setError(null); }} disabled={targetsLoading || channels.length === 0}>
                  <SelectTrigger><SelectValue placeholder={t('models.test.selectChannel')} /></SelectTrigger>
                  <SelectContent>
                    {channels.map((target) => <SelectItem key={target.channelID} value={target.channelID}>{target.channelName}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
            )}
            {channelID && (
              <div className='space-y-2'>
                <Label>{t('models.test.physicalModel')}</Label>
                <Select value={physicalModelID} onValueChange={(value) => { setPhysicalModelID(value); setError(null); }} disabled={physicalModels.length === 0}>
                  <SelectTrigger><SelectValue placeholder={t('models.test.selectPhysicalModel')} /></SelectTrigger>
                  <SelectContent>
                    {physicalModels.map((target) => <SelectItem key={target.physicalModelID} value={target.physicalModelID}>{target.physicalModelID}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
            )}
            {targetsLoading && <p className='text-muted-foreground text-sm'>{t('models.test.loadingTargets')}</p>}
            {targetsError && <p className='text-destructive text-sm'>{t('models.test.targetsError')}</p>}
            {!targetsLoading && !targetsError && protocol && targets.length === 0 && <p className='text-destructive text-sm'>{t('models.test.noTargets')}</p>}
            {protocols.length === 0 && <p className='text-destructive text-sm'>{t('models.test.noProtocol')}</p>}
            {error && <p className='text-destructive rounded-md border border-destructive/30 p-3 text-sm'>{error}</p>}
          </div>
          <DialogFooter>
            <Button variant='ghost' onClick={() => setHistoryOpen(true)}>{t('models.test.history')}</Button>
            <Button variant='outline' onClick={() => onOpenChange(false)}>{t('common.buttons.cancel')}</Button>
            <Button onClick={runTest} disabled={testModel.isPending || !protocol || !channelID || !physicalModelID}>
              {testModel.isPending ? <IconLoader2 className='mr-2 h-4 w-4 animate-spin' /> : <IconPlayerPlay className='mr-2 h-4 w-4' />}
              {testModel.isPending ? t('models.test.testing') : t('models.test.action')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <TestOriginHistoryDrawer open={historyOpen} onOpenChange={setHistoryOpen} originType='model' originID={model.id} label={model.name} />
    </>
  );
}
