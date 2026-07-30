import type { ReactElement } from 'react';
import { createFileRoute } from '@tanstack/react-router';
import { RouteGuard } from '@/components/route-guard';
import AdaptersManagement from '@/features/adapters';

function ProtectedAdapters(): ReactElement {
  return (
    <RouteGuard requiredScopes={['read_channels']} scopeLevel='system'>
      <AdaptersManagement />
    </RouteGuard>
  );
}

export const Route = createFileRoute('/_authenticated/adapters/')({
  component: ProtectedAdapters,
});
