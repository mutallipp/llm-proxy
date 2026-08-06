import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { format } from 'date-fns';
import { ExternalLink, History } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import { useTestOriginHistory } from '../data/requests';

interface TestOriginHistoryDrawerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  originType: 'channel' | 'model' | 'adapter';
  originID: string;
  label?: string;
}

export function TestOriginHistoryDrawer({ open, onOpenChange, originType, originID, label }: TestOriginHistoryDrawerProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [selectedRequestID, setSelectedRequestID] = useState<string | null>(null);
  const { data, isLoading } = useTestOriginHistory(originType, originID, { enabled: open });
  const requests = useMemo(() => data?.edges.map((edge) => edge.node) ?? [], [data]);

  useEffect(() => {
    if (!open) setSelectedRequestID(null);
    else if (!selectedRequestID && requests[0]) setSelectedRequestID(requests[0].id);
  }, [open, requests, selectedRequestID]);

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='w-[95vw] sm:max-w-xl'>
        <SheetHeader>
          <SheetTitle className='flex items-center gap-2'><History className='h-4 w-4' />{t('requests.testOrigin.historyTitle')}</SheetTitle>
          <p className='text-muted-foreground text-sm'>{label ?? originType}</p>
        </SheetHeader>
        <ScrollArea className='mt-4 h-[calc(100vh-8rem)]'>
          {isLoading ? <div className='space-y-3'><Skeleton className='h-16 w-full' /><Skeleton className='h-16 w-full' /></div> : requests.length === 0 ? (
            <p className='text-muted-foreground py-12 text-center text-sm'>{t('requests.testOrigin.empty')}</p>
          ) : (
            <div className='space-y-2 pr-4'>
              {requests.map((request) => (
                <div key={request.id} className={`rounded-md border p-3 ${selectedRequestID === request.id ? 'border-primary bg-muted/40' : ''}`}>
                  <div className='flex items-center justify-between gap-3'>
                    <div className='min-w-0'>
                      <p className='font-mono text-sm'>{request.testOriginLabel || request.modelID}</p>
                      <p className='text-muted-foreground text-xs'>{format(new Date(request.createdAt), 'yyyy-MM-dd HH:mm:ss')}</p>
                    </div>
                    <Badge variant='secondary'>{t(`requests.status.${request.status}`)}</Badge>
                  </div>
                  <Button className='mt-2' size='sm' variant='outline' onClick={async () => {
                    setSelectedRequestID(request.id);
                    onOpenChange(false);
                    await navigate({ to: '/requests/$requestId', params: { requestId: request.id } });
                  }}>
                    <ExternalLink className='mr-1 h-3.5 w-3.5' />{t('requests.testOrigin.viewExecution')}
                  </Button>
                </div>
              ))}
            </div>
          )}
        </ScrollArea>
      </SheetContent>
    </Sheet>
  );
}
