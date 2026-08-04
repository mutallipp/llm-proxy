import { useCallback, useState, type JSX } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { BatteryCharging, Loader2, RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { QuotaRow, groupQuotaChannels } from '@/components/quota-badges';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { checkProviderQuotas, useProviderQuotaStatuses } from '@/features/system/data/quotas';
import { useQuotaEnforcementSettings } from '@/features/system/data/system';
import { CollapsibleSection } from './collapsible-section';

// 在 dashboard 展示各渠道订阅套餐用量（进度条 + 重置倒计时），
// 行内渲染逻辑复用 quota-badges 的 QuotaRow，与顶栏徽章保持同一数据源。
export function ChannelQuotaUsageSection(): JSX.Element | null {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [isRefreshing, setIsRefreshing] = useState(false);
  const { channels, isLoading, isError, error } = useProviderQuotaStatuses();
  const { data: enforcementSettings } = useQuotaEnforcementSettings();
  const enforcementMode = enforcementSettings?.enabled ? enforcementSettings.mode : null;

  const refreshMutation = useMutation({
    mutationFn: async () => {
      return checkProviderQuotas();
    },
    onSuccess: () => {
      void queryClient.refetchQueries({ queryKey: ['provider-quotas'] });
      toast.success(t('system.providerQuota.refresh.success'));
    },
    onError: (refreshError: any) => {
      toast.error(refreshError.message || t('system.providerQuota.refresh.failure'));
    },
  });

  const handleRefresh = useCallback(() => {
    setIsRefreshing(true);
    refreshMutation.mutate(undefined, {
      onSettled: () => setIsRefreshing(false),
    });
  }, [refreshMutation]);

  // 没有支持额度查询的渠道时整块隐藏，避免空区块
  if (!isLoading && !isError && channels.length === 0) return null;

  const groupedChannels = groupQuotaChannels(channels);

  const renderContent = (): JSX.Element => {
    if (isLoading) {
      return (
        <div className='space-y-3'>
          <Skeleton className='h-14 w-full' />
          <Skeleton className='h-14 w-full' />
          <Skeleton className='h-14 w-full' />
        </div>
      );
    }

    if (isError) {
      return (
        <div className='rounded bg-red-500/10 p-2 text-xs break-words text-red-500'>
          <span className='font-medium'>{t('system.providerQuota.error')}:</span>{' '}
          {error instanceof Error ? error.message : t('quota.label.unavailable')}
        </div>
      );
    }

    return (
      <div className={groupedChannels.length > 1 ? 'grid grid-cols-1 gap-x-8 lg:grid-cols-2' : ''}>
        {groupedChannels.map((channel) => (
          <QuotaRow key={channel.id} channel={channel} enforcementMode={enforcementMode} />
        ))}
      </div>
    );
  };

  return (
    <CollapsibleSection
      title={t('dashboard.sections.channelQuota')}
      icon={<BatteryCharging className='text-primary h-4 w-4' />}
      storageKey='channelQuota'
      defaultOpen
    >
      <div className='bg-card rounded-lg border p-4'>
        <div className='mb-2 flex items-center justify-between gap-2'>
          <p className='text-muted-foreground text-sm'>{t('dashboard.sections.channelQuotaDescription')}</p>
          <Button
            variant='ghost'
            size='sm'
            onClick={handleRefresh}
            disabled={isRefreshing || isLoading}
            data-testid='channel-quota-refresh'
          >
            {isRefreshing ? <Loader2 className='h-4 w-4 animate-spin' /> : <RefreshCw className='h-4 w-4' />}
            {t('system.providerQuota.refresh.label')}
          </Button>
        </div>
        {renderContent()}
      </div>
    </CollapsibleSection>
  );
}
