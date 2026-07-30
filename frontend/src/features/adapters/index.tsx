import { useEffect, useState } from 'react';
import { IconEdit, IconPlus, IconRefresh, IconTrash, IconPower } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import { Header } from '@/components/layout/header';
import { Main } from '@/components/layout/main';
import { useQueryChannels } from '@/features/channels/data/channels';
import {
  API_FORMATS,
  type AdapterUpdateInput,
  type GatewayAdapter,
  useAdapters,
  useModelGroups,
  useRefreshGateway,
  useUpsertAdapter,
} from '@/lib/adapterApi';

const EMPTY_ADAPTER: AdapterUpdateInput = {
  display_name: '',
  inbound_api_format: API_FORMATS[0],
  status: 'enabled',
  bindings: [],
};

function statusLabel(status: string) {
  return status === 'enabled' ? '启用' : status === 'disabled' ? '禁用' : '已归档';
}

function statusVariant(status: string): 'default' | 'secondary' | 'destructive' {
  return status === 'enabled' ? 'default' : status === 'disabled' ? 'secondary' : 'destructive';
}

function AdapterDialog({
  adapter,
  open,
  onOpenChange,
}: {
  adapter: GatewayAdapter | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const { data: modelGroupData } = useModelGroups();
  const upsert = useUpsertAdapter();
  const modelGroups = modelGroupData?.model_groups ?? [];
  const [name, setName] = useState('');
  const [draft, setDraft] = useState<AdapterUpdateInput>(EMPTY_ADAPTER);
  const [sourceModelId, setSourceModelId] = useState('');
  const [modelGroupId, setModelGroupId] = useState('');

  useEffect(() => {
    if (open) {
      setName(adapter?.name ?? '');
      setDraft(
        adapter
          ? {
              display_name: adapter.display_name,
              inbound_api_format: adapter.inbound_api_format,
              status: adapter.status,
              remark: adapter.remark,
              bindings: adapter.bindings.map(({ source_model_id, model_group_id, enabled, remark }) => ({
                source_model_id,
                model_group_id,
                enabled,
                remark,
              })),
            }
          : { ...EMPTY_ADAPTER, bindings: [] }
      );
      setSourceModelId('');
      setModelGroupId(modelGroups[0] ? String(modelGroups[0].id) : '');
    }
  }, [adapter, modelGroups, open]);

  useEffect(() => {
    if (open && !modelGroupId && modelGroups[0]) {
      setModelGroupId(String(modelGroups[0].id));
    }
  }, [modelGroupId, modelGroups, open]);

  const addBinding = () => {
    const groupId = Number(modelGroupId);
    if (!sourceModelId.trim() || !groupId) {
      toast.error('请输入源模型并选择模型组');
      return;
    }
    if (draft.bindings.some((binding) => binding.source_model_id === sourceModelId.trim())) {
      toast.error('源模型已存在');
      return;
    }
    setDraft((current) => ({
      ...current,
      bindings: [...current.bindings, { source_model_id: sourceModelId.trim(), model_group_id: groupId, enabled: true }],
    }));
    setSourceModelId('');
  };

  const removeBinding = (index: number) => {
    setDraft((current) => ({ ...current, bindings: current.bindings.filter((_, itemIndex) => itemIndex !== index) }));
  };

  const submit = async () => {
    if (!name.trim()) {
      toast.error('请输入名称');
      return;
    }
    if (!draft.display_name.trim()) {
      toast.error('请输入显示名称');
      return;
    }
    try {
      await upsert.mutateAsync({ name: name.trim(), data: draft });
      toast.success(t('common.buttons.saveChanges', '保存成功'));
      onOpenChange(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '保存失败');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{adapter ? '编辑 Adapter' : '创建 Adapter'}</DialogTitle>
          <DialogDescription>配置入口协议及其逻辑模型绑定。</DialogDescription>
        </DialogHeader>
        <div className='grid gap-4 py-2 sm:grid-cols-2'>
          <div className='grid gap-2'>
            <Label>名称</Label>
            <Input value={name} disabled={Boolean(adapter)} onChange={(event) => setName(event.target.value)} placeholder='例如 pi-anthropic' />
            {!adapter && <p className='text-muted-foreground text-xs'>创建后名称即为 URL 路径，且不可修改。</p>}
          </div>
          <div className='grid gap-2'>
            <Label>显示名称</Label>
            <Input value={draft.display_name} onChange={(event) => setDraft({ ...draft, display_name: event.target.value })} placeholder='例如 Pi Anthropic' />
          </div>
          <div className='grid gap-2'>
            <Label>入站协议</Label>
            <Select value={draft.inbound_api_format} onValueChange={(value) => setDraft({ ...draft, inbound_api_format: value })} disabled={Boolean(adapter)}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>{API_FORMATS.map((format) => <SelectItem key={format} value={format}>{format}</SelectItem>)}</SelectContent>
            </Select>
          </div>
          <div className='grid gap-2'>
            <Label>状态</Label>
            <Select value={draft.status} onValueChange={(value) => setDraft({ ...draft, status: value })}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value='enabled'>启用</SelectItem>
                <SelectItem value='disabled'>禁用</SelectItem>
                <SelectItem value='archived'>归档</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        <div className='space-y-3 border-t pt-4'>
          <div className='flex items-center justify-between'><h3 className='font-medium'>逻辑模型绑定</h3><span className='text-muted-foreground text-sm'>{draft.bindings.length} 个绑定</span></div>
          <div className='grid gap-2 sm:grid-cols-[1fr_1fr_auto]'>
            <Input value={sourceModelId} onChange={(event) => setSourceModelId(event.target.value)} placeholder='源模型 ID，例如 claude-3-7' />
            <Select value={modelGroupId} onValueChange={setModelGroupId}>
              <SelectTrigger><SelectValue placeholder='选择模型组' /></SelectTrigger>
              <SelectContent>{modelGroups.map((group) => <SelectItem key={group.id} value={String(group.id)}>{group.display_name || group.name}</SelectItem>)}</SelectContent>
            </Select>
            <Button type='button' variant='outline' onClick={addBinding}><IconPlus className='mr-1 h-4 w-4' />添加</Button>
          </div>
          <div className='divide-y rounded-md border'>
            {draft.bindings.length === 0 && <p className='text-muted-foreground p-4 text-sm'>暂无绑定</p>}
            {draft.bindings.map((binding, index) => {
              const group = modelGroups.find((item) => item.id === binding.model_group_id);
              return <div className='flex items-center justify-between gap-3 p-3 text-sm' key={`${binding.source_model_id}-${index}`}><span className='font-mono'>{binding.source_model_id}</span><span className='text-muted-foreground flex-1'>{group?.display_name || group?.name || `模型组 #${binding.model_group_id}`}</span><Badge variant={binding.enabled ? 'default' : 'secondary'}>{binding.enabled ? '启用' : '禁用'}</Badge><Button size='icon' variant='ghost' onClick={() => removeBinding(index)} aria-label='删除绑定'><IconTrash className='h-4 w-4' /></Button></div>;
            })}
          </div>
        </div>
        <DialogFooter><Button variant='outline' onClick={() => onOpenChange(false)}>取消</Button><Button onClick={submit} disabled={upsert.isPending}>{upsert.isPending ? '保存中...' : '保存'}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default function AdaptersManagement() {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch } = useAdapters();
  const { data: modelGroupData } = useModelGroups();
  const { data: channelsData } = useQueryChannels({ first: 100, where: { statusIn: ['enabled', 'disabled'] } });
  const refresh = useRefreshGateway();
  const upsert = useUpsertAdapter();
  const [editing, setEditing] = useState<GatewayAdapter | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const adapters = data?.adapters ?? [];
  const modelGroups = modelGroupData?.model_groups ?? [];
  const channelCount = channelsData?.edges?.length ?? 0;

  const openCreate = () => {
    setEditing(null);
    setDialogOpen(true);
  };
  const toggleStatus = async (adapter: GatewayAdapter) => {
    const input: AdapterUpdateInput = {
      display_name: adapter.display_name,
      inbound_api_format: adapter.inbound_api_format,
      status: adapter.status === 'enabled' ? 'disabled' : 'enabled',
      remark: adapter.remark,
      bindings: adapter.bindings.map(({ source_model_id, model_group_id, enabled, remark }) => ({ source_model_id, model_group_id, enabled, remark })),
    };
    try {
      await upsert.mutateAsync({ name: adapter.name, data: input });
      toast.success(input.status === 'enabled' ? 'Adapter 已启用' : 'Adapter 已禁用');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '状态更新失败');
    }
  };

  return <div className='flex flex-1 flex-col overflow-hidden'>
    <Header fixed><div><h1 className='text-xl font-semibold'>Adapter 管理</h1><p className='text-muted-foreground text-sm'>配置消费入口与逻辑模型绑定</p></div><div className='ml-auto flex gap-2'><Button variant='outline' onClick={() => refresh.mutate()} disabled={refresh.isPending}><IconRefresh className='mr-2 h-4 w-4' />刷新快照</Button><Button variant='outline' onClick={() => refetch()}><IconRefresh className='mr-2 h-4 w-4' />重新加载</Button><Button onClick={openCreate}><IconPlus className='mr-2 h-4 w-4' />新建 Adapter</Button></div></Header>
    <Main fixed className='overflow-auto'>
      <Card><CardHeader><CardTitle className='flex items-center justify-between'><span>Adapter 列表</span><span className='text-muted-foreground text-sm font-normal'>{channelCount} 个可用渠道</span></CardTitle></CardHeader><CardContent>
        {isLoading && <p className='text-muted-foreground py-8 text-center'>加载中...</p>}
        {isError && <p className='text-destructive py-8 text-center'>加载失败，请重试。</p>}
        {!isLoading && !isError && <div className='overflow-x-auto'><table className='w-full text-sm'><thead><tr className='border-b text-left'><th className='p-3'>名称</th><th className='p-3'>显示名称</th><th className='p-3'>入站协议</th><th className='p-3'>状态</th><th className='p-3'>逻辑模型数</th><th className='p-3 text-right'>操作</th></tr></thead><tbody>
          {adapters.map((adapter) => <tr className='border-b last:border-0' key={adapter.id || adapter.name}><td className='p-3 font-mono'>{adapter.name}</td><td className='p-3'>{adapter.display_name || '-'}</td><td className='p-3'><Badge variant='outline'>{adapter.inbound_api_format}</Badge></td><td className='p-3'><Badge variant={statusVariant(adapter.status)}>{statusLabel(adapter.status)}</Badge></td><td className='p-3'>{adapter.bindings.length}</td><td className='p-3'><div className='flex justify-end gap-1'><Button size='sm' variant='ghost' onClick={() => { setEditing(adapter); setDialogOpen(true); }}><IconEdit className='mr-1 h-4 w-4' />编辑</Button><Button size='sm' variant='ghost' onClick={() => toggleStatus(adapter)} disabled={upsert.isPending}><IconPower className='mr-1 h-4 w-4' />{adapter.status === 'enabled' ? '禁用' : '启用'}</Button></div></td></tr>)}
          {adapters.length === 0 && <tr><td colSpan={6} className='text-muted-foreground p-8 text-center'>暂无 Adapter，点击右上角创建。</td></tr>}
        </tbody></table></div>}
      </CardContent></Card>
      <p className='text-muted-foreground mt-3 text-xs'>提示：绑定和状态变更会整体提交 Adapter 配置，并自动刷新运行时快照。</p>
    </Main>
    <AdapterDialog adapter={editing} open={dialogOpen} onOpenChange={setDialogOpen} />
  </div>;
}
