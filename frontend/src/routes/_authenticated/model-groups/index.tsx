import type { ReactElement } from 'react';
import { createFileRoute } from '@tanstack/react-router';
import { RouteGuard } from '@/components/route-guard';
import ModelGroupsManagement from '@/features/model-groups';

function ProtectedModelGroups(): ReactElement {
  return (
    <RouteGuard requiredScopes={['read_channels']} scopeLevel='system'>
      <ModelGroupsManagement />
    </RouteGuard>
  );
}

export const Route = createFileRoute('/_authenticated/model-groups/')({
  component: ProtectedModelGroups,
});
