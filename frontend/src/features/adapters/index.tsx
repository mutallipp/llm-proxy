import { useEffect, useState } from 'react';
import { IconEdit, IconFlask, IconLoader2, IconPencil, IconPlus, IconRefresh, IconTrash, IconPower } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Header } from '@/components/layout/header';
import { Main } from '@/components/layout/main';
import { ConfirmDialog } from '@/components/confirm-dialog';
import {
  INBOUND_API_FORMATS,
  type AdapterBinding,
  type AdapterUpdateInput,
  type GatewayAdapter,
  testAdapterBinding,
  useAdapters,
  useDeleteAdapter,
  useModelGroups,
  useRefreshGateway,
  useRenameAdapter,
  useUpsertAdapter,
} from '@/lib/adapterApi';

// ===================== 工具函数 =====================

function statusLabel(status: string) {
  return status === 'enabled' ? '启用' : status === 'disabled' ? '禁用' : '已归档';
}

function statusVariant(status: string): 'default' | 'secondary' | 'destructive' {
  return status === 'enabled' ? 'default' : status === 'disabled' ? 'secondary' : 'destructive';
}

// 绑定行测试状态
type TestState = 'idle' | 'testing' | 'success' | 'failed';

// ===================== 绑定行测试按钮 =====================

interface BindingTestButtonProps {
  adapterName: string;
  inboundApiFormat: string;
  binding: Omit<AdapterBinding, 'id'> & { id?: number };
}

function BindingTestButton({ adapterName, inboundApiFormat, binding }: BindingTestButtonProps) {
  const [state, setState] = useState<TestState>('idle');

  const run = async () => {
    setState('testing');
    try {
      const result = await testAdapterBinding({
        adapterName,
        inboundApiFormat,
        sourceModelId: binding.source_model_id,
      });
      setState('success');
      toast.success(`测试成功（${result.latency.toFixed(2)}s）：${result.summary || '无内容'}`);
    } catch (error) {
      setState('failed');
      toast.error(error instanceof Error ? error.message : '测试失败');
    } finally {
      // 2 秒后重置状态
      setTimeout(() => setState('idle'), 2000);
    }
  };

  return (
    <Button
      size='sm'
      variant={state === 'success' ? 'default' : state === 'failed' ? 'destructive' : 'outline'}
      onClick={run}
      disabled={state === 'testing' || !binding.enabled}
      aria-label={`测试绑定 ${binding.source_model_id}`}
    >
      {state === 'testing' ? (
        <IconLoader2 className='mr-1 h-3 w-3 animate-spin' />
      ) : (
        <IconFlask className='mr-1 h-3 w-3' />
      )}
      {state === 'testing' ? '测试中' : state === 'success' ? '通过' : state === 'failed' ? '失败' : '测试'}
    </Button>
  );
}

// ===================== 新建/编辑 Adapter 弹窗 =====================

const EMPTY_ADAPTER: AdapterUpdateInput = {
  display_name: '',
  inbound_api_format: INBOUND_API_FORMATS[0],
  status: 'enabled',
  bindings: [],
};

interface AdapterDialogProps {
  adapter: GatewayAdapter | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function AdapterDialog({ adapter, open, onOpenChange }: AdapterDialogProps) {
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
    if (draft.bindings.some((b) => b.source_model_id === sourceModelId.trim())) {
      toast.error('源模型已存在');
      return;
    }
    setDraft((cur) => ({
      ...cur,
      bindings: [...cur.bindings, { source_model_id: sourceModelId.trim(), model_group_id: groupId, enabled: true }],
    }));
    setSourceModelId('');
  };

  const removeBinding = (index: number) => {
    setDraft((cur) => ({ ...cur, bindings: cur.bindings.filter((_, i) => i !== index) }));
  };

  const submit = async () => {
    if (!name.trim()) { toast.error('请输入名称'); return; }
    if (!draft.display_name.trim()) { toast.error('请输入显示名称'); return; }
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
            <Input
              value={name}
              disabled={Boolean(adapter)}
              onChange={(e) => setName(e.target.value)}
              placeholder='例如 pi-anthropic'
            />
            {!adapter && (
              <p className='text-muted-foreground text-xs'>创建后名称即为 URL 路径，且不可修改（可使用"重命名"操作）。</p>
            )}
          </div>
          <div className='grid gap-2'>
            <Label>显示名称</Label>
            <Input
              value={draft.display_name}
              onChange={(e) => setDraft({ ...draft, display_name: e.target.value })}
              placeholder='例如 Pi Anthropic'
            />
          </div>
          <div className='grid gap-2'>
            <Label>入站协议</Label>
            <Select
              value={draft.inbound_api_format}
              onValueChange={(value) => setDraft({ ...draft, inbound_api_format: value })}
              disabled={Boolean(adapter)}
            >
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {INBOUND_API_FORMATS.map((f) => <SelectItem key={f} value={f}>{f}</SelectItem>)}
              </SelectContent>
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

        {/* 逻辑模型绑定区 */}
        <div className='space-y-3 border-t pt-4'>
          <div className='flex items-center justify-between'>
            <h3 className='font-medium'>逻辑模型绑定</h3>
            <span className='text-muted-foreground text-sm'>{draft.bindings.length} 个绑定</span>
          </div>
          <div className='grid gap-2 sm:grid-cols-[1fr_1fr_auto]'>
            <Input
              value={sourceModelId}
              onChange={(e) => setSourceModelId(e.target.value)}
              placeholder='源模型 ID，例如 claude-3-7'
            />
            <Select value={modelGroupId} onValueChange={setModelGroupId}>
              <SelectTrigger><SelectValue placeholder='选择模型组' /></SelectTrigger>
              <SelectContent>
                {modelGroups.map((g) => (
                  <SelectItem key={g.id} value={String(g.id)}>{g.display_name || g.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button type='button' variant='outline' onClick={addBinding}>
              <IconPlus className='mr-1 h-4 w-4' />添加
            </Button>
          </div>
          <div className='divide-y rounded-md border'>
            {draft.bindings.length === 0 && (
              <p className='text-muted-foreground p-4 text-sm'>暂无绑定</p>
            )}
            {draft.bindings.map((binding, index) => {
              const group = modelGroups.find((g) => g.id === binding.model_group_id);
              return (
                <div
                  key={`${binding.source_model_id}-${index}`}
                  className='flex flex-wrap items-center justify-between gap-2 p-3 text-sm'
                >
                  <span className='font-mono'>{binding.source_model_id}</span>
                  <span className='text-muted-foreground flex-1'>
                    {group?.display_name || group?.name || `模型组 #${binding.model_group_id}`}
                  </span>
                  <Badge variant={binding.enabled ? 'default' : 'secondary'}>
                    {binding.enabled ? '启用' : '禁用'}
                  </Badge>
                  {/* 已保存的 adapter 显示测试按钮 */}
                  {adapter && (
                    <BindingTestButton
                      adapterName={adapter.name}
                      inboundApiFormat={adapter.inbound_api_format}
                      binding={binding}
                    />
                  )}
                  <Button
                    size='icon'
                    variant='ghost'
                    onClick={() => removeBinding(index)}
                    aria-label='删除绑定'
                  >
                    <IconTrash className='h-4 w-4' />
                  </Button>
                </div>
              );
            })}
          </div>
          {adapter && draft.bindings.length > 0 && (
            <p className='text-muted-foreground text-xs'>
              点击「测试」可通过 Adapter 入口发送最小请求验证整条链路是否畅通。
            </p>
          )}
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>取消</Button>
          <Button onClick={submit} disabled={upsert.isPending}>
            {upsert.isPending ? '保存中...' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ===================== 重命名 Adapter 弹窗 =====================

interface RenameDialogProps {
  adapter: GatewayAdapter;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function RenameAdapterDialog({ adapter, open, onOpenChange }: RenameDialogProps) {
  const rename = useRenameAdapter();
  const [newName, setNewName] = useState('');

  useEffect(() => {
    if (open) setNewName(adapter.name);
  }, [adapter.name, open]);

  const submit = async () => {
    const trimmed = newName.trim();
    if (!trimmed) { toast.error('请输入新名称'); return; }
    if (trimmed === adapter.name) { onOpenChange(false); return; }
    try {
      await rename.mutateAsync({ name: adapter.name, newName: trimmed });
      toast.success(`Adapter 已重命名为 "${trimmed}"`);
      onOpenChange(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '重命名失败');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-md'>
        <DialogHeader>
          <DialogTitle>重命名 Adapter</DialogTitle>
          <DialogDescription>
            当前名称：<span className='font-mono'>{adapter.name}</span>
          </DialogDescription>
        </DialogHeader>
        <div className='grid gap-3 py-2'>
          <Label>新名称</Label>
          <Input
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder='输入新的 Adapter 名称'
            onKeyDown={(e) => e.key === 'Enter' && submit()}
          />
          <p className='text-destructive text-xs'>
            ⚠️ 重命名后，旧的消费 URL（<code>/{adapter.name}/v1/...</code>）将立即失效，所有使用该 URL 的客户端需同步更新。
          </p>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>取消</Button>
          <Button onClick={submit} disabled={rename.isPending}>
            {rename.isPending ? '重命名中...' : '确认重命名'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ===================== 主页面 =====================

export default function AdaptersManagement() {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch } = useAdapters();
  const { data: modelGroupData } = useModelGroups();
  const refresh = useRefreshGateway();
  const upsert = useUpsertAdapter();
  const deleteAdapter = useDeleteAdapter();

  // 编辑弹窗状态
  const [editing, setEditing] = useState<GatewayAdapter | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);

  // 重命名弹窗状态
  const [renaming, setRenaming] = useState<GatewayAdapter | null>(null);
  const [renameOpen, setRenameOpen] = useState(false);

  // 删除确认弹窗状态
  const [deleting, setDeleting] = useState<GatewayAdapter | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);

  const adapters = data?.adapters ?? [];
  const modelGroups = modelGroupData?.model_groups ?? [];

  const openCreate = () => {
    setEditing(null);
    setDialogOpen(true);
  };

  const openRename = (adapter: GatewayAdapter) => {
    setRenaming(adapter);
    setRenameOpen(true);
  };

  const openDelete = (adapter: GatewayAdapter) => {
    setDeleting(adapter);
    setDeleteOpen(true);
  };

  const toggleStatus = async (adapter: GatewayAdapter) => {
    const input: AdapterUpdateInput = {
      display_name: adapter.display_name,
      inbound_api_format: adapter.inbound_api_format,
      status: adapter.status === 'enabled' ? 'disabled' : 'enabled',
      remark: adapter.remark,
      bindings: adapter.bindings.map(({ source_model_id, model_group_id, enabled, remark }) => ({
        source_model_id, model_group_id, enabled, remark,
      })),
    };
    try {
      await upsert.mutateAsync({ name: adapter.name, data: input });
      toast.success(input.status === 'enabled' ? 'Adapter 已启用' : 'Adapter 已禁用');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '状态更新失败');
    }
  };

  const confirmDelete = async () => {
    if (!deleting) return;
    try {
      await deleteAdapter.mutateAsync({ name: deleting.name });
      toast.success(`Adapter "${deleting.name}" 已删除`);
      setDeleteOpen(false);
      setDeleting(null);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '删除失败');
    }
  };

  // 仅用于展示模型组名称
  const groupNames = new Map(modelGroups.map((g) => [g.id, g.display_name || g.name]));

  return (
    <div className='flex flex-1 flex-col overflow-hidden'>
      <Header fixed>
        <div>
          <h1 className='text-xl font-semibold'>Adapter 管理</h1>
          <p className='text-muted-foreground text-sm'>配置消费入口与逻辑模型绑定</p>
        </div>
        <div className='ml-auto flex gap-2'>
          <Button variant='outline' onClick={() => refresh.mutate()} disabled={refresh.isPending}>
            <IconRefresh className='mr-2 h-4 w-4' />刷新快照
          </Button>
          <Button variant='outline' onClick={() => refetch()}>
            <IconRefresh className='mr-2 h-4 w-4' />重新加载
          </Button>
          <Button onClick={openCreate}>
            <IconPlus className='mr-2 h-4 w-4' />新建 Adapter
          </Button>
        </div>
      </Header>

      <Main fixed className='overflow-auto'>
        <Card>
          <CardHeader>
            <CardTitle>Adapter 列表</CardTitle>
          </CardHeader>
          <CardContent>
            {isLoading && <p className='text-muted-foreground py-8 text-center'>加载中...</p>}
            {isError && <p className='text-destructive py-8 text-center'>加载失败，请重试。</p>}
            {!isLoading && !isError && (
              <div className='overflow-x-auto'>
                <table className='w-full text-sm'>
                  <thead>
                    <tr className='border-b text-left'>
                      <th className='p-3'>名称</th>
                      <th className='p-3'>显示名称</th>
                      <th className='p-3'>入站协议</th>
                      <th className='p-3'>状态</th>
                      <th className='p-3 text-right'>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {adapters.map((adapter) => (
                      <tr key={adapter.id || adapter.name} className='border-b last:border-0'>
                        <td className='p-3 font-mono'>{adapter.name}</td>
                        <td className='p-3'>{adapter.display_name || '-'}</td>
                        <td className='p-3'>
                          <Badge variant='outline'>{adapter.inbound_api_format}</Badge>
                        </td>
                        <td className='p-3'>
                          <Badge variant={statusVariant(adapter.status)}>{statusLabel(adapter.status)}</Badge>
                        </td>
                        <td className='p-3'>
                          <div className='flex flex-wrap justify-end gap-1'>
                            <Button
                              size='sm'
                              variant='ghost'
                              onClick={() => { setEditing(adapter); setDialogOpen(true); }}
                            >
                              <IconEdit className='mr-1 h-4 w-4' />编辑
                            </Button>
                            <Button
                              size='sm'
                              variant='ghost'
                              onClick={() => openRename(adapter)}
                            >
                              <IconPencil className='mr-1 h-4 w-4' />重命名
                            </Button>
                            <Button
                              size='sm'
                              variant='ghost'
                              onClick={() => toggleStatus(adapter)}
                              disabled={upsert.isPending}
                            >
                              <IconPower className='mr-1 h-4 w-4' />
                              {adapter.status === 'enabled' ? '禁用' : '启用'}
                            </Button>
                            <Button
                              size='sm'
                              variant='ghost'
                              className='text-destructive hover:text-destructive'
                              onClick={() => openDelete(adapter)}
                            >
                              <IconTrash className='mr-1 h-4 w-4' />删除
                            </Button>
                          </div>
                        </td>
                      </tr>
                    ))}
                    {adapters.length === 0 && (
                      <tr>
                        <td colSpan={5} className='text-muted-foreground p-8 text-center'>
                          暂无 Adapter，点击右上角创建。
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            )}
          </CardContent>
        </Card>

        {/* 展开视图：每个 adapter 下的绑定列表（带测试按钮） */}
        {!isLoading && !isError && adapters.length > 0 && (
          <div className='mt-4 space-y-4'>
            {adapters.map((adapter) => (
              <Card key={adapter.id || adapter.name}>
                <CardHeader className='flex flex-row items-center justify-between space-y-0 pb-2'>
                  <CardTitle className='text-base'>
                    <span className='font-mono'>{adapter.name}</span>
                    <Badge variant='outline' className='ml-2'>{adapter.inbound_api_format}</Badge>
                  </CardTitle>
                  <span className='text-muted-foreground text-sm'>{adapter.bindings.length} 个绑定</span>
                </CardHeader>
                <CardContent>
                  {adapter.bindings.length === 0 ? (
                    <p className='text-muted-foreground text-sm'>暂无绑定</p>
                  ) : (
                    <div className='divide-y rounded-md border'>
                      {adapter.bindings.map((binding, index) => (
                        <div
                          key={`${binding.source_model_id}-${index}`}
                          className='flex flex-wrap items-center gap-3 p-3 text-sm'
                        >
                          <span className='font-mono font-medium'>{binding.source_model_id}</span>
                          <span className='text-muted-foreground flex-1'>
                            → {groupNames.get(binding.model_group_id) ?? `模型组 #${binding.model_group_id}`}
                          </span>
                          <Badge variant={binding.enabled ? 'default' : 'secondary'}>
                            {binding.enabled ? '启用' : '禁用'}
                          </Badge>
                          <BindingTestButton
                            adapterName={adapter.name}
                            inboundApiFormat={adapter.inbound_api_format}
                            binding={binding}
                          />
                        </div>
                      ))}
                    </div>
                  )}
                </CardContent>
              </Card>
            ))}
          </div>
        )}

        <p className='text-muted-foreground mt-3 text-xs'>
          提示：绑定和状态变更会整体提交 Adapter 配置，并自动刷新运行时快照。
        </p>
      </Main>

      {/* 创建/编辑弹窗 */}
      <AdapterDialog adapter={editing} open={dialogOpen} onOpenChange={setDialogOpen} />

      {/* 重命名弹窗 */}
      {renaming && (
        <RenameAdapterDialog
          adapter={renaming}
          open={renameOpen}
          onOpenChange={(open) => {
            setRenameOpen(open);
            if (!open) setRenaming(null);
          }}
        />
      )}

      {/* 删除确认弹窗 */}
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title='删除 Adapter'
        desc={
          deleting ? (
            <span>
              确定要删除 Adapter <strong className='font-mono'>{deleting.name}</strong> 吗？
              <br />
              <span className='text-destructive text-xs'>
                ⚠️ 删除后，消费 URL（<code>/{deleting.name}/v1/...</code>）将立即失效，该操作不可撤销。
              </span>
            </span>
          ) : ''
        }
        confirmText='确认删除'
        cancelBtnText='取消'
        destructive
        isLoading={deleteAdapter.isPending}
        handleConfirm={confirmDelete}
      />
    </div>
  );
}
