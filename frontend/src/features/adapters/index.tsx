import { useEffect, useMemo, useState } from 'react';
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
import { useQueryAllModels } from '@/features/models/data/models';
import type { Model } from '@/features/models/data/schema';
import { extractNumberIDAsNumber } from '@/lib/utils';
import {
  INBOUND_API_FORMATS,
  type AdapterBinding,
  type AdapterTestResult,
  type AdapterUpdateInput,
  type GatewayAdapter,
  testAdapterBinding,
  useAdapters,
  useDeleteAdapter,
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

const EMPTY_MODELS: Model[] = [];
const NUMERIC_MODEL_ID_PATTERN = /^[1-9]\d*$/;
const MODEL_RELAY_GID_PATTERN = /^gid:\/\/llm-proxy\/Model\/[1-9]\d*$/;

function parseModelIdFromSelectValue(value: string): number | null {
  const isNumericId = NUMERIC_MODEL_ID_PATTERN.test(value);
  const isRelayGid = MODEL_RELAY_GID_PATTERN.test(value);
  if (!isNumericId && !isRelayGid) return null;

  const modelId = extractNumberIDAsNumber(value);
  return Number.isSafeInteger(modelId) && modelId > 0 ? modelId : null;
}

function modelIdToSelectValue(modelId: number | null | undefined, models: readonly Model[]): string {
  if (!Number.isSafeInteger(modelId) || modelId <= 0) return '';
  return models.find((model) => parseModelIdFromSelectValue(model.id) === modelId)?.id ?? '';
}

function findModelById(models: readonly Model[], modelId: number): Model | undefined {
  return models.find((model) => parseModelIdFromSelectValue(model.id) === modelId);
}

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
      if (!result.ok) {
        // 非 2xx：显示错误
        setState('failed');
        toast.error(result.error ?? `HTTP ${result.status}`);
      } else {
        setState('success');
        toast.success(`测试成功（${result.latency.toFixed(2)}s）：${result.summary || '无内容'}`);
      }
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

// ===================== curl 预览生成 =====================

/**
 * 根据入站协议和源模型 ID 生成与实际请求一致的 curl 命令预览。
 * 不含 Authorization/API Key，仅展示入站协议和请求体结构。
 */
function buildCurl(adapterName: string, inboundApiFormat: string, sourceModelId: string): string {
  const origin = window.location.origin;
  const encodedName = encodeURIComponent(adapterName);

  if (inboundApiFormat === 'anthropic/messages') {
    const bodyObj = {
      model: sourceModelId,
      max_tokens: 16,
      messages: [{ role: 'user', content: 'Reply with OK.' }],
      stream: false,
    };
    return [
      `curl -X POST '${origin}/${encodedName}/v1/messages' \\`,
      `  -H 'Content-Type: application/json' \\`,
      `  -d '${JSON.stringify(bodyObj)}'`,
    ].join('\n');
  } else if (inboundApiFormat === 'openai/responses') {
    const bodyObj = {
      model: sourceModelId,
      input: 'Reply with OK.',
      max_output_tokens: 16,
      stream: false,
    };
    return [
      `curl -X POST '${origin}/${encodedName}/v1/responses' \\`,
      `  -H 'Content-Type: application/json' \\`,
      `  -d '${JSON.stringify(bodyObj)}'`,
    ].join('\n');
  } else {
    // openai/chat_completions（默认）
    const bodyObj = {
      model: sourceModelId,
      max_tokens: 16,
      messages: [{ role: 'user', content: 'Reply with OK.' }],
      stream: false,
    };
    return [
      `curl -X POST '${origin}/${encodedName}/v1/chat/completions' \\`,
      `  -H 'Content-Type: application/json' \\`,
      `  -d '${JSON.stringify(bodyObj)}'`,
    ].join('\n');
  }
}

// ===================== Adapter 测试弹窗 =====================

interface AdapterTestDialogProps {
  adapter: GatewayAdapter | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function AdapterTestDialog({ adapter, open, onOpenChange }: AdapterTestDialogProps) {
  const { data: modelsData } = useQueryAllModels({});
  const modelEdges = modelsData?.edges;
  const models = useMemo(
    () => modelEdges?.map(({ node }) => node) ?? EMPTY_MODELS,
    [modelEdges]
  );

  // 只展示已启用的绑定项
  const enabledBindings = adapter?.bindings.filter((b) => b.enabled) ?? [];

  const [selectedIndex, setSelectedIndex] = useState(0);
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState<AdapterTestResult | null>(null);

  // 弹窗打开时重置状态
  useEffect(() => {
    if (open) {
      setSelectedIndex(0);
      setResult(null);
    }
  }, [open]);

  if (!adapter) return null;

  const selectedBinding = enabledBindings[selectedIndex];

  // 切换绑定时清除上次结果
  const handleSelectBinding = (value: string) => {
    setSelectedIndex(Number(value));
    setResult(null);
  };

  const getModelName = (modelId: number) => {
    const model = findModelById(models, modelId);
    return model?.name || model?.modelID || `模型 #${modelId}`;
  };

  // 当前绑定的 curl 预览
  const curlPreview = selectedBinding
    ? buildCurl(adapter.name, adapter.inbound_api_format, selectedBinding.source_model_id)
    : '';

  // 尝试将原始响应 JSON 格式化
  const formattedResponse = (() => {
    if (!result?.rawResponse) return '';
    try {
      return JSON.stringify(JSON.parse(result.rawResponse), null, 2);
    } catch {
      return result.rawResponse;
    }
  })();

  const runTest = async () => {
    if (!selectedBinding || testing) return;
    setTesting(true);
    setResult(null);
    try {
      const res = await testAdapterBinding({
        adapterName: adapter.name,
        inboundApiFormat: adapter.inbound_api_format,
        sourceModelId: selectedBinding.source_model_id,
      });
      setResult(res);
      if (res.ok) {
        toast.success(`测试成功（${res.latency.toFixed(2)}s）`);
      } else {
        toast.error(res.error ?? `HTTP ${res.status}`);
      }
    } catch (err) {
      // testAdapterBinding 已捕获网络错误，这里仅作兴底处理
      toast.error(err instanceof Error ? err.message : '测试失败');
    } finally {
      setTesting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(o) => { if (!testing) onOpenChange(o); }}>
      <DialogContent className='w-[96vw] max-w-[1000px] max-h-[90vh] overflow-y-auto'>
        <DialogHeader>
          <DialogTitle>测试 Adapter：{adapter.display_name || adapter.name}</DialogTitle>
          <DialogDescription>
            通过 Adapter 入站协议发送最小请求，验证整条链路连通性。
          </DialogDescription>
        </DialogHeader>

        {enabledBindings.length === 0 ? (
          // 没有已启用绑定时的提示
          <div className='py-8 text-center'>
            <p className='text-muted-foreground'>该 Adapter 暂无可用的已启用绑定，请先在编辑中添加并启用绑定。</p>
          </div>
        ) : (
          <div className='space-y-4'>
            {/* 绑定选择器 */}
            <div className='space-y-2'>
              <Label>选择绑定</Label>
              <Select value={String(selectedIndex)} onValueChange={handleSelectBinding}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {enabledBindings.map((binding, i) => (
                    <SelectItem key={`${binding.source_model_id}-${i}`} value={String(i)}>
                      {binding.source_model_id} · {getModelName(binding.model_id)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className='text-muted-foreground text-xs'>
                测试使用真实 Adapter 入站协议和逆辑模型，不直接测试目标模型 ID。
              </p>
            </div>

            {/* curl 预览 */}
            {curlPreview && (
              <div className='space-y-2'>
                <Label>将要执行的请求（curl 预览）</Label>
                <pre className='bg-muted text-muted-foreground overflow-x-auto rounded-md p-3 text-xs leading-relaxed whitespace-pre-wrap break-all'>
                  {curlPreview}
                </pre>
              </div>
            )}

            {/* 执行测试按钮 */}
            <Button onClick={runTest} disabled={testing || !selectedBinding} className='w-full'>
              {testing ? (
                <>
                  <IconLoader2 className='mr-2 h-4 w-4 animate-spin' />
                  测试中...
                </>
              ) : (
                <>
                  <IconFlask className='mr-2 h-4 w-4' />
                  执行测试
                </>
              )}
            </Button>

            {/* 测试结果展示区 */}
            {result && (
              <div className='space-y-3 rounded-md border p-4'>
                {/* 状态和耗时 */}
                <div className='flex flex-wrap items-center gap-4'>
                  <div className='flex items-center gap-2'>
                    <span className='text-sm font-medium'>状态：</span>
                    <Badge variant={result.ok ? 'default' : 'destructive'}>
                      {result.status > 0 ? `HTTP ${result.status}` : '网络错误'}
                    </Badge>
                  </div>
                  <div className='flex items-center gap-2'>
                    <span className='text-sm font-medium'>耗时：</span>
                    <span className='text-muted-foreground text-sm'>{result.latency.toFixed(2)}s</span>
                  </div>
                </div>

                {/* 错误信息 */}
                {result.error && (
                  <div className='space-y-1'>
                    <p className='text-sm font-medium text-destructive'>错误信息</p>
                    <p className='text-destructive text-sm'>{result.error}</p>
                  </div>
                )}

                {/* 响应内容 */}
                {formattedResponse && (
                  <div className='space-y-1'>
                    <p className='text-sm font-medium'>响应内容</p>
                    <pre className='bg-muted max-h-64 overflow-y-auto overflow-x-auto rounded-md p-3 text-xs whitespace-pre-wrap break-all'>
                      <code>{formattedResponse}</code>
                    </pre>
                  </div>
                )}
              </div>
            )}
          </div>
        )}

        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)} disabled={testing}>关闭</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
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
  const upsert = useUpsertAdapter();
  const { data: modelsData } = useQueryAllModels({});
  const modelEdges = modelsData?.edges;
  const models = useMemo(
    () => modelEdges?.map(({ node }) => node) ?? EMPTY_MODELS,
    [modelEdges]
  );
  const firstModelId = models[0]
    ? modelIdToSelectValue(parseModelIdFromSelectValue(models[0].id), models)
    : '';
  const [name, setName] = useState('');
  const [draft, setDraft] = useState<AdapterUpdateInput>(EMPTY_ADAPTER);
  const [sourceModelId, setSourceModelId] = useState('');
  const [modelId, setModelId] = useState('');

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
              bindings: adapter.bindings.map(({ source_model_id, model_id, enabled, remark }) => ({
                source_model_id,
                model_id,
                enabled,
                remark,
              })),
            }
          : { ...EMPTY_ADAPTER, bindings: [] }
      );
      setSourceModelId('');
      setModelId('');
    }
  }, [adapter, open]);

  useEffect(() => {
    if (open && !modelId && firstModelId) {
      setModelId(firstModelId);
    }
  }, [firstModelId, modelId, open]);

  const addBinding = () => {
    const selectedModelId = parseModelIdFromSelectValue(modelId);
    if (!sourceModelId.trim() || selectedModelId === null) {
      toast.error('请输入源模型并选择模型');
      return;
    }
    if (draft.bindings.some((b) => b.source_model_id === sourceModelId.trim())) {
      toast.error('源模型已存在');
      return;
    }
    setDraft((cur) => ({
      ...cur,
      bindings: [...cur.bindings, { source_model_id: sourceModelId.trim(), model_id: selectedModelId, enabled: true }],
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
            <Select value={modelId} onValueChange={setModelId}>
              <SelectTrigger><SelectValue placeholder='选择模型' /></SelectTrigger>
              <SelectContent>
                {models.map((model) => (
                  <SelectItem key={model.id} value={String(model.id)}>{model.name || model.modelID}</SelectItem>
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
              const model = findModelById(models, binding.model_id);
              return (
                <div
                  key={`${binding.source_model_id}-${index}`}
                  className='flex flex-wrap items-center justify-between gap-2 p-3 text-sm'
                >
                  <span className='font-mono'>{binding.source_model_id}</span>
                  <span className='text-muted-foreground flex-1'>
                    {model?.name || model?.modelID || `模型 #${binding.model_id}`}
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

  // 测试弹窗状态
  const [testTarget, setTestTarget] = useState<GatewayAdapter | null>(null);
  const [isTestOpen, setIsTestOpen] = useState(false);

  const adapters = data?.adapters ?? [];

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
      bindings: adapter.bindings.map(({ source_model_id, model_id, enabled, remark }) => ({
        source_model_id, model_id, enabled, remark,
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
                      <th className='p-3'>绑定数</th>
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
                          <span className='text-muted-foreground'>{adapter.bindings.length} 个绑定</span>
                        </td>
                        <td className='p-3'>
                          <Badge variant={statusVariant(adapter.status)}>{statusLabel(adapter.status)}</Badge>
                        </td>
                        <td className='p-3'>
                          <div className='flex flex-wrap justify-end gap-1'>
                            {/* 列表行测试按钮：非启用状态时 disabled 并显示提示 */}
                            <span title={adapter.status !== 'enabled' ? '请先启用 Adapter' : undefined}>
                              <Button
                                size='sm'
                                variant='ghost'
                                onClick={() => { setTestTarget(adapter); setIsTestOpen(true); }}
                                disabled={adapter.status !== 'enabled'}
                              >
                                <IconFlask className='mr-1 h-4 w-4' />测试
                              </Button>
                            </span>
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
                        <td colSpan={6} className='text-muted-foreground p-8 text-center'>
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

        <p className='text-muted-foreground mt-3 text-xs'>
          提示：绑定和状态变更会整体提交 Adapter 配置，并自动刷新运行时快照。
        </p>
      </Main>

      {/* 测试弹窗 */}
      <AdapterTestDialog
        adapter={testTarget}
        open={isTestOpen}
        onOpenChange={(o) => {
          setIsTestOpen(o);
          if (!o) setTestTarget(null);
        }}
      />

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
