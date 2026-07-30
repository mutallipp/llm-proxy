import { useEffect, useMemo, useState } from 'react';
import { IconArrowDown, IconArrowUp, IconEdit, IconFlask, IconLoader2, IconPlus, IconRefresh, IconTrash } from '@tabler/icons-react';
import { AlertTriangle } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { Header } from '@/components/layout/header';
import { Main } from '@/components/layout/main';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { useQueryChannels, useTestChannel } from '@/features/channels/data/channels';
import {
  API_FORMATS,
  INBOUND_API_FORMATS,
  type ModelGroup,
  type ModelGroupProtocol,
  type ModelGroupTargetInput,
  type ModelGroupUpdateInput,
  useDeleteModelGroup,
  useModelGroups,
  useRefreshGateway,
  useUpsertModelGroup,
} from '@/lib/adapterApi';

// 空目标的 capabilities 默认值
const EMPTY_CAPABILITIES = {
  supports_tools: false,
  supports_stream: true,
  input_modalities: ['text'],
  output_modalities: ['text'],
};

// 新建模型组时的初始值
const EMPTY_GROUP: ModelGroupUpdateInput = {
  display_name: '',
  status: 'enabled',
  selection_strategy: 'priority_failover',
  protocols: [],
};

// 添加目标时的临时表单状态类型
type NewTargetDraft = {
  channel_id: string;
  target_model_id: string;
  outbound_api_format: string;
  enabled: boolean;
};

// 对 targets 列表重新按下标赋 priority（从 1 开始）
function normalizeTargetPriorities(targets: ModelGroupTargetInput[]): ModelGroupTargetInput[] {
  return targets.map((target, index) => ({ ...target, priority: index + 1 }));
}

// status -> 中文标签
function statusLabel(status: string): string {
  if (status === 'enabled') return '启用';
  if (status === 'disabled') return '禁用';
  return '已归档';
}

// status -> Badge 颜色
function statusVariant(status: string): 'default' | 'secondary' | 'destructive' {
  if (status === 'enabled') return 'default';
  if (status === 'disabled') return 'secondary';
  return 'destructive';
}

// 从渠道 endpoints + defaultEndpoints 中提取可用 apiFormat 列表
// 若渠道未配置任何 endpoint，则返回全部 API_FORMATS 作为后备
function getChannelAvailableFormats(
  channelId: string | number,
  channels: Array<{ id: string; endpoints?: Array<{ apiFormat: string }> | null; defaultEndpoints?: Array<{ apiFormat: string }> | null }>,
): string[] {
  const ch = channels.find((c) => String(c.id) === String(channelId));
  if (!ch) return [...API_FORMATS];
  const fromEndpoints = [
    ...(ch.endpoints ?? []),
    ...(ch.defaultEndpoints ?? []),
  ].map((e) => e.apiFormat).filter(Boolean);
  if (fromEndpoints.length === 0) return [...API_FORMATS];
  return [...new Set(fromEndpoints)];
}

// ===================== ModelGroupTarget 渠道测试按钮 =====================

type TargetTestState = 'idle' | 'testing' | 'success' | 'failed';

interface TargetTestButtonProps {
  channelId: number;
  targetModelId: string;
}

function TargetTestButton({ channelId, targetModelId }: TargetTestButtonProps) {
  const [state, setState] = useState<TargetTestState>('idle');
  const testChannel = useTestChannel({ silent: true });

  const run = async () => {
    if (!targetModelId.trim()) { toast.error('目标模型 ID 不能为空'); return; }
    setState('testing');
    try {
      const result = await testChannel.mutateAsync({
        channelID: String(channelId),
        modelID: targetModelId.trim(),
      });
      setState('success');
      if (result.success) {
        toast.success(`渠道目标测试通过（${result.latency.toFixed(2)}s）${result.message ? '：' + result.message : ''}`);
      } else {
        setState('failed');
        toast.error(result.error || '渠道目标测试失败');
      }
    } catch (error) {
      setState('failed');
      toast.error(error instanceof Error ? error.message : '测试失败');
    } finally {
      setTimeout(() => setState('idle'), 2000);
    }
  };

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size='sm'
            variant={state === 'success' ? 'default' : state === 'failed' ? 'destructive' : 'outline'}
            className='h-8 px-2'
            onClick={run}
            disabled={state === 'testing'}
            aria-label={`测试渠道 #${channelId} 目标 ${targetModelId}`}
          >
            {state === 'testing' ? (
              <IconLoader2 className='h-3 w-3 animate-spin' />
            ) : (
              <IconFlask className='h-3 w-3' />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          <p>渠道目标测试（仅验证该渠道直连，非 Adapter 全链路）</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

// ===================== 编辑弹窗 =====================

interface GroupDialogProps {
  group: ModelGroup | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function GroupDialog({ group, open, onOpenChange }: GroupDialogProps) {
  const { data: channelsData } = useQueryChannels({ first: 100, where: { statusIn: ['enabled', 'disabled'] } });
  const upsert = useUpsertModelGroup();
  const channels = channelsData?.edges?.map((edge) => edge.node) ?? [];

  // 表单基础字段
  const [name, setName] = useState('');
  const [draft, setDraft] = useState<ModelGroupUpdateInput>(EMPTY_GROUP);

  // 新增协议时选择的入站格式
  const [newProtocolFormat, setNewProtocolFormat] = useState<string>(INBOUND_API_FORMATS[0]);

  // 每个协议对应的"新增目标"草稿，key 为协议下标
  const [newTargets, setNewTargets] = useState<Record<number, NewTargetDraft>>({});

  // 渠道 id -> 渠道名称映射（用于目标列表显示）
  const channelNames = useMemo(
    () => new Map(channels.map((ch) => [Number(ch.id), ch.name])),
    [channels],
  );

  // 生成一个空的新目标草稿（默认选第一个渠道）
  const makeTargetDraft = (): NewTargetDraft => ({
    channel_id: channels[0]?.id ?? '',
    target_model_id: '',
    outbound_api_format: API_FORMATS[0],
    enabled: true,
  });

  // 弹窗打开时重置所有状态
  useEffect(() => {
    if (!open) return;
    setName(group?.name ?? '');
    setDraft(
      group
        ? {
            display_name: group.display_name,
            status: group.status,
            selection_strategy: group.selection_strategy,
            remark: group.remark,
            protocols: group.protocols.map(({ inbound_api_format, enabled, remark, targets }) => ({
              inbound_api_format,
              enabled,
              remark,
              targets: targets.map((target) => ({
                channel_id: target.channel_id,
                target_model_id: target.target_model_id,
                outbound_api_format: target.outbound_api_format,
                priority: target.priority,
                enabled: target.enabled,
                remark: target.remark,
                capabilities: { ...target.capabilities },
              })),
            })),
          }
        : { ...EMPTY_GROUP, protocols: [] },
    );
    setNewProtocolFormat(INBOUND_API_FORMATS[0]);
    setNewTargets({});
  }, [group, open]);

  // ---- 协议操作 ----

  const addProtocol = () => {
    if (draft.protocols.some((p) => p.inbound_api_format === newProtocolFormat)) {
      toast.error('该入站协议已存在');
      return;
    }
    setDraft((current) => ({
      ...current,
      protocols: [
        ...current.protocols,
        { inbound_api_format: newProtocolFormat, enabled: true, targets: [] },
      ],
    }));
  };

  const removeProtocol = (index: number) => {
    setDraft((current) => ({
      ...current,
      protocols: current.protocols.filter((_, i) => i !== index),
    }));
  };

  const updateProtocol = (index: number, patch: Partial<ModelGroupProtocol>) => {
    setDraft((current) => ({
      ...current,
      protocols: current.protocols.map((p, i) => (i === index ? { ...p, ...patch } : p)),
    }));
  };

  // ---- 目标操作 ----

  const addTarget = (protocolIndex: number) => {
    const target = newTargets[protocolIndex] ?? makeTargetDraft();
    if (!Number(target.channel_id) || !target.target_model_id.trim()) {
      toast.error('请选择渠道并填写目标模型 ID');
      return;
    }
    setDraft((current) => ({
      ...current,
      protocols: current.protocols.map((p, i) => {
        if (i !== protocolIndex) return p;
        return {
          ...p,
          targets: [
            ...p.targets,
            {
              channel_id: Number(target.channel_id),
              target_model_id: target.target_model_id.trim(),
              outbound_api_format: target.outbound_api_format,
              priority: p.targets.length + 1,
              enabled: target.enabled,
              capabilities: { ...EMPTY_CAPABILITIES },
            },
          ],
        };
      }),
    }));
    // 重置该协议的新目标草稿（保留已选渠道）
    setNewTargets((current) => ({
      ...current,
      [protocolIndex]: { ...makeTargetDraft(), channel_id: target.channel_id },
    }));
  };

  const removeTarget = (protocolIndex: number, targetIndex: number) => {
    setDraft((current) => ({
      ...current,
      protocols: current.protocols.map((p, i) => {
        if (i !== protocolIndex) return p;
        return {
          ...p,
          targets: normalizeTargetPriorities(p.targets.filter((_, ti) => ti !== targetIndex)),
        };
      }),
    }));
  };

  const updateTarget = (protocolIndex: number, targetIndex: number, patch: Partial<ModelGroupTargetInput>) => {
    setDraft((current) => ({
      ...current,
      protocols: current.protocols.map((p, i) => {
        if (i !== protocolIndex) return p;
        return {
          ...p,
          targets: p.targets.map((t, ti) => (ti === targetIndex ? { ...t, ...patch } : t)),
        };
      }),
    }));
  };

  const moveTarget = (protocolIndex: number, targetIndex: number, direction: -1 | 1) => {
    setDraft((current) => ({
      ...current,
      protocols: current.protocols.map((p, i) => {
        if (i !== protocolIndex) return p;
        const nextIndex = targetIndex + direction;
        if (nextIndex < 0 || nextIndex >= p.targets.length) return p;
        const targets = [...p.targets];
        [targets[targetIndex], targets[nextIndex]] = [targets[nextIndex], targets[targetIndex]];
        return { ...p, targets: normalizeTargetPriorities(targets) };
      }),
    }));
  };

  // 更新某协议的新目标草稿
  const updateNewTarget = (protocolIndex: number, patch: Partial<NewTargetDraft>) => {
    setNewTargets((current) => {
      const base = current[protocolIndex] ?? makeTargetDraft();
      return { ...current, [protocolIndex]: { ...base, ...patch } };
    });
  };

  // ---- 提交 ----

  const submit = async () => {
    if (!name.trim()) {
      toast.error('请输入模型组名称');
      return;
    }
    if (!draft.display_name.trim()) {
      toast.error('请输入显示名称');
      return;
    }
    try {
      await upsert.mutateAsync({ name: name.trim(), data: draft });
      toast.success('模型组保存成功');
      onOpenChange(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '保存失败');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='w-[96vw] max-w-[1400px] max-h-[90vh] overflow-y-auto'>
        <DialogHeader>
          <DialogTitle>{group ? '编辑模型组' : '创建模型组'}</DialogTitle>
          <DialogDescription>配置模型组支持的入站协议及其目标池。</DialogDescription>
        </DialogHeader>

        {/* 基础信息 */}
        <div className='grid gap-4 py-2 sm:grid-cols-3'>
          <div className='grid gap-2'>
            <Label>名称</Label>
            <Input
              value={name}
              disabled={Boolean(group)}
              onChange={(e) => setName(e.target.value)}
              placeholder='例如 claude-primary'
            />
            {!group && <p className='text-muted-foreground text-xs'>创建后不可修改。</p>}
          </div>
          <div className='grid gap-2'>
            <Label>显示名称</Label>
            <Input
              value={draft.display_name}
              onChange={(e) => setDraft({ ...draft, display_name: e.target.value })}
              placeholder='例如 Claude 主力组'
            />
          </div>
          <div className='grid gap-2'>
            <Label>状态</Label>
            <Select value={draft.status} onValueChange={(value) => setDraft({ ...draft, status: value })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='enabled'>启用</SelectItem>
                <SelectItem value='disabled'>禁用</SelectItem>
                <SelectItem value='archived'>归档</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        {/* 选择策略 */}
        <div className='grid gap-2 sm:max-w-xs'>
          <Label>选择策略</Label>
          <Select
            value={draft.selection_strategy}
            onValueChange={(value) => setDraft({ ...draft, selection_strategy: value })}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='priority_failover'>按优先级故障转移</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {/* 协议管理 */}
        <div className='space-y-4 border-t pt-4'>
          {/* 多协议使用说明 */}
          <div className='bg-muted/40 rounded-md border px-4 py-3 text-sm'>
            <p className='font-medium mb-1'>多协议配置说明</p>
            <p className='text-muted-foreground'>
              同一渠道可以支持多种出站协议（如同时支持 OpenAI 和 Anthropic 格式）。
              请先在 <strong>渠道管理 → Endpoints</strong> 中为该渠道配置多个不同协议的 Endpoint，
              再在下方的目标行中选择对应的出站协议（outbound API format）。
              出站格式下拉会根据所选渠道的 Endpoint 自动过滤。
            </p>
          </div>
          <div className='flex items-center gap-2'>
            <h3 className='font-medium'>协议管理</h3>
            <div className='ml-auto flex items-center gap-2'>
              <Select value={newProtocolFormat} onValueChange={setNewProtocolFormat}>
                <SelectTrigger className='w-64'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {INBOUND_API_FORMATS.map((format) => (
                    <SelectItem key={format} value={format}>
                      {format}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button type='button' variant='outline' onClick={addProtocol}>
                <IconPlus className='mr-1 h-4 w-4' />
                添加协议
              </Button>
            </div>
          </div>

          {draft.protocols.length === 0 && (
            <p className='text-muted-foreground rounded-md border p-4 text-sm'>
              暂无协议，请添加至少一个入站协议。
            </p>
          )}

          {draft.protocols.map((protocol, protocolIndex) => {
            const targetDraft = newTargets[protocolIndex] ?? makeTargetDraft();
            return (
              <Card key={`${protocol.inbound_api_format}-${protocolIndex}`}>
                <CardHeader className='flex flex-row items-center justify-between space-y-0 pb-3'>
                  <CardTitle className='text-base'>
                    <Badge variant='outline'>{protocol.inbound_api_format}</Badge>
                  </CardTitle>
                  <div className='flex items-center gap-2'>
                    {/* 协议启用/禁用 */}
                    <Select
                      value={protocol.enabled ? 'enabled' : 'disabled'}
                      onValueChange={(value) =>
                        updateProtocol(protocolIndex, { enabled: value === 'enabled' })
                      }
                    >
                      <SelectTrigger className='h-8 w-24'>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value='enabled'>启用</SelectItem>
                        <SelectItem value='disabled'>禁用</SelectItem>
                      </SelectContent>
                    </Select>
                    <Button
                      size='icon'
                      variant='ghost'
                      onClick={() => removeProtocol(protocolIndex)}
                      aria-label='删除协议'
                    >
                      <IconTrash className='h-4 w-4' />
                    </Button>
                  </div>
                </CardHeader>

                <CardContent className='space-y-3'>
                  {/* 新增目标表单 */}
                  <div className='grid gap-2 rounded-md bg-muted/40 p-3 sm:grid-cols-[1fr_1fr_1fr_80px_80px_auto]'>
                    {/* 选择渠道 */}
                    <Select
                      value={targetDraft.channel_id}
                      onValueChange={(value) => updateNewTarget(protocolIndex, { channel_id: value })}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder='选择渠道' />
                      </SelectTrigger>
                      <SelectContent>
                        {channels.map((ch) => (
                          <SelectItem key={ch.id} value={ch.id}>
                            {ch.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    {/* 目标模型 ID */}
                    <Input
                      value={targetDraft.target_model_id}
                      onChange={(e) => updateNewTarget(protocolIndex, { target_model_id: e.target.value })}
                      placeholder='目标模型 ID'
                    />
                    {/* 出站格式：根据所选渠道的 endpoints 过滤 */}
                    {(() => {
                      const availableFormats = getChannelAvailableFormats(targetDraft.channel_id, channels);
                      return (
                        <Select
                          value={targetDraft.outbound_api_format}
                          onValueChange={(value) => updateNewTarget(protocolIndex, { outbound_api_format: value })}
                        >
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {availableFormats.map((format) => (
                              <SelectItem key={format} value={format}>
                                {format}
                              </SelectItem>
                            ))}
                            {/* 当可用格式不包含当前值时，展示保留项 */}
                            {!availableFormats.includes(targetDraft.outbound_api_format) && (
                              <SelectItem key={targetDraft.outbound_api_format} value={targetDraft.outbound_api_format}>
                                {targetDraft.outbound_api_format} (警告)
                              </SelectItem>
                            )}
                          </SelectContent>
                        </Select>
                      );
                    })()}
                    {/* 启用/禁用 */}
                    <Select
                      value={targetDraft.enabled ? 'enabled' : 'disabled'}
                      onValueChange={(value) => updateNewTarget(protocolIndex, { enabled: value === 'enabled' })}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value='enabled'>启用</SelectItem>
                        <SelectItem value='disabled'>禁用</SelectItem>
                      </SelectContent>
                    </Select>
                    <div className='col-span-2 flex justify-end'>
                      <Button
                        type='button'
                        variant='outline'
                        className='w-full'
                        onClick={() => addTarget(protocolIndex)}
                      >
                        <IconPlus className='mr-1 h-4 w-4' />
                        添加
                      </Button>
                    </div>
                  </div>

                  {/* 目标列表 */}
                  <div className='divide-y rounded-md border'>
                    {protocol.targets.length === 0 && (
                      <p className='text-muted-foreground p-3 text-sm'>暂无目标</p>
                    )}
                    {protocol.targets.map((target, targetIndex) => {
                      // 该渠道的可用出站协议列表
                      const availableFormats = getChannelAvailableFormats(target.channel_id, channels);
                      // 若当前保存的 outbound_api_format 不在可用列表中，显示警告
                      const isFormatMismatch = availableFormats.length > 0 &&
                        !availableFormats.includes(target.outbound_api_format);
                      return (
                      <div
                        key={`${target.id ?? 'new'}-${targetIndex}`}
                        className='grid items-center gap-2 p-3 text-sm sm:grid-cols-[1fr_1fr_1fr_60px_80px_auto]'
                      >
                        {/* 渠道名 */}
                        <span className='truncate text-muted-foreground'>
                          {channelNames.get(target.channel_id) ?? `渠道 #${target.channel_id}`}
                        </span>
                        {/* 目标模型 ID（可编辑） */}
                        <Input
                          value={target.target_model_id}
                          onChange={(e) =>
                            updateTarget(protocolIndex, targetIndex, { target_model_id: e.target.value })
                          }
                          className='h-8'
                        />
                        {/* 出站格式（可编辑），根据渠道 endpoints 过滤 */}
                        <div className='flex items-center gap-1'>
                          {isFormatMismatch && (
                            <TooltipProvider>
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <AlertTriangle className='text-destructive h-4 w-4 shrink-0' />
                                </TooltipTrigger>
                                <TooltipContent>
                                  <p>当前出站协议 {target.outbound_api_format} 不在该渠道已配置的 Endpoint 列表中，请检查。</p>
                                </TooltipContent>
                              </Tooltip>
                            </TooltipProvider>
                          )}
                          <Select
                            value={target.outbound_api_format}
                            onValueChange={(value) =>
                              updateTarget(protocolIndex, targetIndex, { outbound_api_format: value })
                            }
                          >
                            <SelectTrigger className='h-8 flex-1'>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {availableFormats.map((format) => (
                                <SelectItem key={format} value={format}>
                                  {format}
                                </SelectItem>
                              ))}
                              {/* 保留不在列表中的当前值，避免静默丢失 */}
                              {isFormatMismatch && (
                                <SelectItem key={target.outbound_api_format} value={target.outbound_api_format}>
                                  {target.outbound_api_format} ⚠️
                                </SelectItem>
                              )}
                            </SelectContent>
                          </Select>
                        </div>
                        {/* 优先级（可编辑） */}
                        <Input
                          type='number'
                          min={1}
                          value={target.priority}
                          onChange={(e) =>
                            updateTarget(protocolIndex, targetIndex, { priority: Number(e.target.value) || 1 })
                          }
                          className='h-8 text-center'
                        />
                        {/* 启用状态（可编辑） */}
                        <Select
                          value={target.enabled ? 'enabled' : 'disabled'}
                          onValueChange={(value) =>
                            updateTarget(protocolIndex, targetIndex, { enabled: value === 'enabled' })
                          }
                        >
                          <SelectTrigger className='h-8'>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value='enabled'>启用</SelectItem>
                            <SelectItem value='disabled'>禁用</SelectItem>
                          </SelectContent>
                        </Select>
                        {/* 操作按钮 */}
                        <div className='flex justify-end gap-1'>
                          {/* 渠道目标测试（仅验证直连，非完整 Adapter 链路） */}
                          <TargetTestButton
                            channelId={target.channel_id}
                            targetModelId={target.target_model_id}
                          />
                          <Button
                            size='icon'
                            variant='ghost'
                            className='h-8 w-8'
                            disabled={targetIndex === 0}
                            onClick={() => moveTarget(protocolIndex, targetIndex, -1)}
                            aria-label='上移'
                          >
                            <IconArrowUp className='h-4 w-4' />
                          </Button>
                          <Button
                            size='icon'
                            variant='ghost'
                            className='h-8 w-8'
                            disabled={targetIndex === protocol.targets.length - 1}
                            onClick={() => moveTarget(protocolIndex, targetIndex, 1)}
                            aria-label='下移'
                          >
                            <IconArrowDown className='h-4 w-4' />
                          </Button>
                          <Button
                            size='icon'
                            variant='ghost'
                            className='h-8 w-8'
                            onClick={() => removeTarget(protocolIndex, targetIndex)}
                            aria-label='删除目标'
                          >
                            <IconTrash className='h-4 w-4' />
                          </Button>
                        </div>
                      </div>
                      );
                    })}
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={submit} disabled={upsert.isPending}>
            {upsert.isPending ? '保存中...' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ===================== 主页面 =====================

export default function ModelGroupsManagement() {
  const { data, isLoading, isError, refetch } = useModelGroups();
  const refresh = useRefreshGateway();
  const deleteModelGroup = useDeleteModelGroup();

  // 编辑弹窗状态
  const [editing, setEditing] = useState<ModelGroup | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);

  // 删除确认弹窗状态
  const [deleting, setDeleting] = useState<ModelGroup | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);

  const groups = data?.model_groups ?? [];

  const openCreate = () => {
    setEditing(null);
    setDialogOpen(true);
  };

  const openEdit = (group: ModelGroup) => {
    setEditing(group);
    setDialogOpen(true);
  };

  const openDelete = (group: ModelGroup) => {
    setDeleting(group);
    setDeleteOpen(true);
  };

  const confirmDelete = async () => {
    if (!deleting) return;
    try {
      await deleteModelGroup.mutateAsync({ name: deleting.name });
      toast.success(`模型组 "${deleting.name}" 已删除`);
      setDeleteOpen(false);
      setDeleting(null);
    } catch (error) {
      // 如果返回 409，说明仍有 Adapter 绑定
      const msg = error instanceof Error ? error.message : '删除失败';
      if (msg.includes('409') || msg.toLowerCase().includes('conflict') || msg.includes('bound') || msg.includes('绑定')) {
        toast.error('该模型组仍有 Adapter 绑定，请先在 Adapter 管理页面移除相关绑定，再删除模型组');
      } else {
        toast.error(msg);
      }
    }
  };

  return (
    <div className='flex flex-1 flex-col overflow-hidden'>
      {/* 顶部标题栏 */}
      <Header fixed>
        <div>
          <h1 className='text-xl font-semibold'>ModelGroup 管理</h1>
          <p className='text-muted-foreground text-sm'>管理可复用模型组、协议和目标池</p>
        </div>
        <div className='ml-auto flex gap-2'>
          <Button
            variant='outline'
            onClick={() => refresh.mutate()}
            disabled={refresh.isPending}
          >
            <IconRefresh className='mr-2 h-4 w-4' />
            刷新快照
          </Button>
          <Button variant='outline' onClick={() => refetch()}>
            <IconRefresh className='mr-2 h-4 w-4' />
            重新加载
          </Button>
          <Button onClick={openCreate}>
            <IconPlus className='mr-2 h-4 w-4' />
            新建模型组
          </Button>
        </div>
      </Header>

      {/* 主内容区 */}
      <Main fixed className='overflow-auto'>
        <Card>
          <CardHeader>
            <CardTitle>ModelGroup 列表</CardTitle>
          </CardHeader>
          <CardContent>
            {isLoading && (
              <p className='text-muted-foreground py-8 text-center'>加载中...</p>
            )}
            {isError && (
              <p className='text-destructive py-8 text-center'>加载失败，请重试。</p>
            )}
            {!isLoading && !isError && (
              <div className='overflow-x-auto'>
                <table className='w-full text-sm'>
                  <thead>
                    <tr className='border-b text-left'>
                      <th className='p-3'>名称</th>
                      <th className='p-3'>显示名称</th>
                      <th className='p-3'>状态</th>
                      <th className='p-3'>协议数</th>
                      <th className='p-3'>目标数</th>
                      <th className='p-3 text-right'>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {groups.map((group) => {
                      // 计算全部协议下的目标总数
                      const targetCount = group.protocols.reduce(
                        (total, protocol) => total + protocol.targets.length,
                        0,
                      );
                      return (
                        <tr className='border-b last:border-0' key={group.id || group.name}>
                          <td className='p-3 font-mono'>{group.name}</td>
                          <td className='p-3'>{group.display_name || '-'}</td>
                          <td className='p-3'>
                            <Badge variant={statusVariant(group.status)}>
                              {statusLabel(group.status)}
                            </Badge>
                          </td>
                          <td className='p-3'>{group.protocols.length}</td>
                          <td className='p-3'>{targetCount}</td>
                          <td className='p-3 text-right'>
                            <div className='flex justify-end gap-1'>
                              <Button size='sm' variant='ghost' onClick={() => openEdit(group)}>
                                <IconEdit className='mr-1 h-4 w-4' />
                                编辑
                              </Button>
                              <Button
                                size='sm'
                                variant='ghost'
                                className='text-destructive hover:text-destructive'
                                onClick={() => openDelete(group)}
                              >
                                <IconTrash className='mr-1 h-4 w-4' />
                                删除
                              </Button>
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                    {groups.length === 0 && (
                      <tr>
                        <td
                          colSpan={6}
                          className='text-muted-foreground p-8 text-center'
                        >
                          暂无模型组，点击右上角「新建模型组」开始配置。
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            )}
          </CardContent>
        </Card>

        <p className='text-muted-foreground mt-3 text-xs'>
          目标优先级可通过上下箭头调整，保存时整体更新协议配置。
        </p>
      </Main>

      {/* 创建 / 编辑弹窗 */}
      <GroupDialog group={editing} open={dialogOpen} onOpenChange={setDialogOpen} />

      {/* 删除确认弹窗 */}
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title='删除模型组'
        desc={
          deleting ? (
            <span>
              确定要删除模型组 <strong className='font-mono'>{deleting.name}</strong> 吗？
              <br />
              <span className='text-destructive text-xs'>
                ⚠️ 如果仍有 Adapter 绑定该模型组，删除将会失败。请先移除所有绑定再删除。
              </span>
            </span>
          ) : ''
        }
        confirmText='确认删除'
        cancelBtnText='取消'
        destructive
        isLoading={deleteModelGroup.isPending}
        handleConfirm={confirmDelete}
      />
    </div>
  );
}
