import { useModels } from '../context/models-context';
import { ModelsActionDialog } from './models-action-dialog';
import { ModelsArchiveDialog } from './models-archive-dialog';
import { ModelsBatchCreateDialog } from './models-batch-create-dialog';
import { ModelsBulkDisableDialog } from './models-bulk-disable-dialog';
import { ModelsBulkEnableDialog } from './models-bulk-enable-dialog';
import { ModelsDeleteDialog } from './models-delete-dialog';
import { ModelSettingsDialog } from './models-settings-dialog';
import { ModelsUnassociatedDialog } from './models-unassociated-dialog';
import { ModelsTestDialog } from './models-test-dialog';

export function ModelsDialogs() {
  const { open, currentRow, setOpen } = useModels();

  return (
    <>
      {(open === 'create' || open === 'edit') && <ModelsActionDialog />}
      {open === 'batchCreate' && <ModelsBatchCreateDialog />}
      {open === 'delete' && <ModelsDeleteDialog />}
      {open === 'archive' && <ModelsArchiveDialog />}
      {open === 'settings' && <ModelSettingsDialog />}
      {open === 'unassociated' && <ModelsUnassociatedDialog />}
      {open === 'test' && currentRow && <ModelsTestDialog model={currentRow} open onOpenChange={(value) => setOpen(value ? 'test' : null)} />}
      <ModelsBulkDisableDialog />
      <ModelsBulkEnableDialog />
    </>
  );
}
