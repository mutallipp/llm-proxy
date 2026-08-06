import { TestOriginHistoryDrawer } from '@/features/requests/components/test-origin-history-drawer';
import { Channel } from '../data/schema';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channel: Channel;
}

export function ChannelsTestHistoryDrawer({ open, onOpenChange, channel }: Props) {
  return (
    <TestOriginHistoryDrawer
      open={open}
      onOpenChange={onOpenChange}
      originType='channel'
      originID={channel.id}
      label={channel.name}
    />
  );
}
