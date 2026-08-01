import { useEffect, useState, useMemo, useCallback } from 'react';
import { format } from 'date-fns';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { toc } from '@lobehub/icons';
import { CalendarIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';
import { formatNumber } from '@/utils/format-number';
import { Button } from '@/components/ui/button';
import { Calendar } from '@/components/ui/calendar';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { AutoComplete } from '@/components/auto-complete';
import { AutoCompleteSelect } from '@/components/auto-complete-select';
import { useModels } from '../context/models-context';
import { useQueryChannels } from '@/features/channels/data/channels';
import type { ModelProtocolPool } from '../data/schema';
import { DEVELOPER_IDS, DEVELOPER_ICONS } from '../data/constants';
import { useCreateModel, useUpdateModel } from '../data/models';
import { useDevelopersData } from '../data/providers';
import { type Provider, type ProviderModel, resolveVision } from '../data/providers.schema';
import { CreateModelInput, createModelInputSchema, UpdateModelInput, ModelCard, ModelType, modelTypeSchema, updateModelInputSchema } from '../data/schema';

function isDeveloper(provider: string) {
  return DEVELOPER_IDS.includes(provider);
}

export function ModelsActionDialog() {
  const { t } = useTranslation();
  const { open, setOpen, currentRow } = useModels();
  const createModel = useCreateModel();
  const updateModel = useUpdateModel();
  const { data: developersData } = useDevelopersData();
  const [selectedProvider, setSelectedProvider] = useState<string>('');
  const [developerSearchValue, setDeveloperSearchValue] = useState<string>('');
  const [modelIdInput, setModelIdInput] = useState<string>('');
  const [modelIdSearchValue, setModelIdSearchValue] = useState<string>('');
  const [_selectedModelCard, setSelectedModelCard] = useState<ModelCard>({});

  // 用于解决 Dialog 内 Popover 无法滚动的问题
  const [dialogContent, setDialogContent] = useState<HTMLDivElement | null>(null);
  const [protocolPools, setProtocolPools] = useState<ModelProtocolPool[]>([]);
  const [protocolError, setProtocolError] = useState('');
  const { data: channelsData } = useQueryChannels({ first: 2000 });
  const channels = channelsData?.edges?.map((edge) => edge.node) ?? [];

  const isEdit = open === 'edit';
  const isOpen = open === 'create' || open === 'edit';

  const providers = useMemo(() => {
    if (!developersData) return [];
    return Object.entries(developersData.providers)
      .filter(([key]) => isDeveloper(key))
      .map(([key, provider]: [string, Provider]) => ({
        id: key,
        name: provider.display_name || provider.name,
        models: provider.models || [],
      }));
  }, [developersData]);

  const selectedProviderModels = useMemo(() => {
    if (!selectedProvider) return [];
    const provider = providers.find((p) => p.id === selectedProvider);
    return provider?.models || [];
  }, [selectedProvider, providers]);

  const developerOptions = useMemo(() => {
    return DEVELOPER_IDS.map((id) => ({
      value: id,
      label: id,
    }));
  }, []);

  const modelIdOptions = useMemo(() => {
    return selectedProviderModels.map((m: ProviderModel) => ({
      value: m.id,
      label: m.id,
    }));
  }, [selectedProviderModels]);

  const iconOptions = useMemo(() => {
    return (
      Object.entries(toc)
        // @ts-ignore
        .filter(([_, value]) => value.group == 'provider' || value.group == 'model')
        .map(([_, value]) => ({
          // @ts-ignore
          value: value.id,
          // @ts-ignore
          label: value.id,
        }))
    );
  }, []);

  const form = useForm<CreateModelInput>({
    resolver: zodResolver(isEdit ? updateModelInputSchema : createModelInputSchema) as any,
    defaultValues: {
      developer: '',
      modelID: '',
      type: 'chat',
      name: '',
      icon: '',
      group: '',
      modelCard: {},
      settings: { protocolPools: [] },
      remark: '',
    },
  });

  useEffect(() => {
    if (isEdit && currentRow) {
      form.reset({
        developer: currentRow.developer,
        modelID: currentRow.modelID,
        type: currentRow.type,
        name: currentRow.name,
        icon: currentRow.icon,
        group: currentRow.group,
        modelCard: currentRow.modelCard,
        settings: currentRow.settings,
        remark: currentRow.remark || '',
      });
      setSelectedProvider(currentRow.developer);
      setDeveloperSearchValue(currentRow.developer);
      setModelIdInput(currentRow.modelID);
      setModelIdSearchValue(currentRow.modelID);
      setSelectedModelCard(currentRow.modelCard || {});
      setProtocolPools(currentRow.settings?.protocolPools || []);
    } else if (!isEdit) {
      form.reset({
        developer: '',
        modelID: '',
        type: 'chat',
        name: '',
        icon: '',
        group: '',
        modelCard: {},
        settings: { protocolPools: [] },
        remark: '',
      });
      setSelectedProvider('');
      setDeveloperSearchValue('');
      setModelIdInput('');
      setModelIdSearchValue('');
      setSelectedModelCard({});
      setProtocolPools([]);
    }
  }, [isEdit, currentRow, form, isOpen]);

  const handleProviderChange = useCallback(
    (providerId: string) => {
      setSelectedProvider(providerId);
      setDeveloperSearchValue(providerId);
      form.setValue('developer', providerId);
      if (!isEdit) {
        const icon = DEVELOPER_ICONS[providerId] || providerId;
        form.setValue('icon', icon);
        setModelIdInput('');
        setModelIdSearchValue('');
        form.setValue('modelID', '');
        form.setValue('name', '');
        form.setValue('group', '');
        form.setValue('modelCard', {});
        setSelectedModelCard({});
      }
    },
    [form, isEdit]
  );

  const handleModelIdChange = useCallback(
    (modelId: string) => {
      setModelIdInput(modelId);
      setModelIdSearchValue(modelId);
      form.setValue('modelID', modelId);

      const selectedModel = selectedProviderModels.find((m: ProviderModel) => m.id === modelId);

      if (selectedModel && !isEdit) {
        form.setValue('name', selectedModel.display_name || selectedModel.name || '');
        form.setValue('group', selectedModel.family || selectedProvider);
        const normalizedType = selectedModel.type?.replace(/-/g, '_');
        if (normalizedType && modelTypeSchema.safeParse(normalizedType).success) {
          form.setValue('type', normalizedType as ModelType);
        }
        const modelCard: ModelCard = {
          reasoning: {
            supported: selectedModel.reasoning?.supported || false,
            default: selectedModel.reasoning?.default || false,
          },
          toolCall: selectedModel.tool_call,
          temperature: selectedModel.temperature,
          modalities: {
            input: selectedModel.modalities?.input || [],
            output: selectedModel.modalities?.output || [],
          },
          vision: resolveVision(selectedModel),
          cost: {
            input: selectedModel.cost?.input || 0,
            output: selectedModel.cost?.output || 0,
            cacheRead: selectedModel.cost?.cache_read,
            cacheWrite: selectedModel.cost?.cache_write,
          },
          limit: {
            context: selectedModel.limit?.context || 0,
            output: selectedModel.limit?.output || 0,
          },
          knowledge: selectedModel.knowledge,
          releaseDate: selectedModel.release_date,
          lastUpdated: selectedModel.last_updated,
        };
        form.setValue('modelCard', modelCard);
        setSelectedModelCard(modelCard);
      } else {
        const currentModelCard = form.getValues('modelCard');
        setSelectedModelCard(currentModelCard || {});
      }
    },
    [selectedProviderModels, selectedProvider, form, isEdit]
  );

  const onSubmit = async (data: CreateModelInput) => {
    if (isEdit) {
      if (protocolPools.some((pool) => !pool.format.trim())) { setProtocolError('协议池格式不能为空'); return; }
      for (const pool of protocolPools) {
        if (pool.associations.length === 0) { setProtocolError(`协议池 ${pool.format} 至少需要一个 target`); return; }
        const keys = pool.associations.map((target) => `${target.channelModel?.channelId || 0}:${target.channelModel?.modelId?.trim() || ''}`);
        if (keys.some((key) => key.startsWith('0:') || key.endsWith(':'))) { setProtocolError('每个 target 都必须选择渠道并填写物理模型'); return; }
        if (new Set(keys).size !== keys.length) { setProtocolError(`协议池 ${pool.format} 存在重复的渠道和物理模型`); return; }
      }
      setProtocolError('');
    }
    try {
      if (isEdit && currentRow) {
        const updateData: UpdateModelInput = {
          developer: data.developer,
          modelID: data.modelID,
          type: data.type,
          name: data.name,
          icon: data.icon,
          group: data.group,
          modelCard: data.modelCard,
          settings: { ...data.settings, protocolPools },
          remark: data.remark,
        };
        await updateModel.mutateAsync({ id: currentRow.id, input: updateData });
      } else {
        await createModel.mutateAsync(data);
      }
      handleClose();
    } catch (error) {
      const status = typeof error === 'object' && error !== null && 'status' in error ? Number(error.status) : 0;
      setProtocolError(status === 401 ? '未登录或登录已过期' : status === 403 ? '没有修改模型的权限' : status === 404 ? '模型或渠道不存在' : status === 409 ? '协议池配置冲突，请检查重复 target' : status >= 500 ? '服务暂时不可用，请稍后重试' : '保存协议池失败，请检查输入后重试');
    }
  };

  const handleClose = useCallback(() => {
    setOpen(null);
    form.reset();
    setSelectedProvider('');
    setDeveloperSearchValue('');
    setModelIdInput('');
    setModelIdSearchValue('');
    setSelectedModelCard({});
  }, [form, setOpen]);

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent ref={setDialogContent} className='flex max-h-[90vh] flex-col overflow-hidden sm:max-w-6xl'>
        <DialogHeader className='flex-shrink-0 text-left'>
          <DialogTitle>{isEdit ? t('models.dialogs.edit.title') : t('models.dialogs.create.title')}</DialogTitle>
          <DialogDescription>{isEdit ? t('models.dialogs.edit.description') : t('models.dialogs.create.description')}</DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form id='model-form' onSubmit={form.handleSubmit(onSubmit)} className='flex min-h-0 flex-1 flex-col overflow-hidden'>
            <div className='flex min-h-0 flex-1 gap-6 overflow-x-auto overflow-y-hidden md:overflow-hidden'>
              {/* Left Panel - Basic Information */}
              <div className='min-h-0 w-1/2 md:w-1/3 flex-shrink-0 overflow-y-auto pr-4'>
                <div className='space-y-4'>
                  <FormField
                    control={form.control}
                    name='developer'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('models.fields.developer')}</FormLabel>
                        <FormControl>
                          <AutoComplete
                            selectedValue={selectedProvider}
                            onSelectedValueChange={handleProviderChange}
                            searchValue={developerSearchValue}
                            onSearchValueChange={setDeveloperSearchValue}
                            items={developerOptions}
                            placeholder={t('models.fields.selectDeveloper')}
                            emptyMessage={t('models.fields.noModels')}
                            portalContainer={dialogContent}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='modelID'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('models.fields.modelId')}</FormLabel>
                        <FormControl>
                          {selectedProvider && modelIdOptions.length > 0 ? (
                            <AutoComplete
                              selectedValue={modelIdInput}
                              onSelectedValueChange={handleModelIdChange}
                              searchValue={modelIdSearchValue}
                              onSearchValueChange={setModelIdSearchValue}
                              items={modelIdOptions}
                              placeholder={t('models.fields.modelIdPlaceholder')}
                              emptyMessage={t('models.fields.noModels')}
                              portalContainer={dialogContent}
                            />
                          ) : (
                            <AutoComplete
                              selectedValue={modelIdInput}
                              onSelectedValueChange={handleModelIdChange}
                              searchValue={modelIdSearchValue}
                              onSearchValueChange={setModelIdSearchValue}
                              items={[]}
                              placeholder={t('models.fields.modelIdPlaceholder')}
                              emptyMessage={t('models.fields.noModels')}
                              portalContainer={dialogContent}
                            />
                          )}
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='name'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('models.fields.name')}</FormLabel>
                        <FormControl>
                          <Input {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='icon'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('models.fields.icon')}</FormLabel>
                        <FormControl>
                          <AutoCompleteSelect
                            selectedValue={field.value}
                            onSelectedValueChange={field.onChange}
                            items={iconOptions}
                            placeholder={t('models.fields.selectIcon')}
                            emptyMessage={t('models.fields.noIcons')}
                            portalContainer={dialogContent}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='group'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('models.fields.group')}</FormLabel>
                        <FormControl>
                          <Input {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='type'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('models.fields.type')}</FormLabel>
                        <Select value={field.value} onValueChange={field.onChange}>
                          <FormControl>
                            <SelectTrigger>
                              <SelectValue />
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent>
                            <SelectItem value='chat'>{t('models.types.chat')}</SelectItem>
                            <SelectItem value='embedding'>{t('models.types.embedding')}</SelectItem>
                            <SelectItem value='rerank'>{t('models.types.rerank')}</SelectItem>
                            <SelectItem value='image_generation'>{t('models.types.image_generation')}</SelectItem>
                            <SelectItem value='video_generation'>{t('models.types.video_generation')}</SelectItem>
                          </SelectContent>
                        </Select>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='remark'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('models.fields.remark')}</FormLabel>
                        <FormControl>
                          <Textarea {...field} value={field.value || ''} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              </div>

              {/* Right Panel - Model Card Fields */}
              <div className='min-h-0 min-w-full md:min-w-0 flex-1 overflow-y-auto border-l pl-6'>
                <div className='space-y-4 pb-4'>
                  <h3 className='text-lg font-semibold'>{t('models.modelCard.title')}</h3>

                  <div className='space-y-2'>
                    <FormLabel>{t('models.modelCard.capabilities')}</FormLabel>
                    <div className='grid grid-cols-2 gap-2'>
                      <FormField
                        control={form.control}
                        name='modelCard.toolCall'
                        render={({ field }) => (
                          <FormItem className='flex items-center space-y-0 space-x-2'>
                            <FormControl>
                              <Checkbox checked={field.value || false} onCheckedChange={field.onChange} />
                            </FormControl>
                            <FormLabel className='font-normal'>{t('models.modelCard.toolCall')}</FormLabel>
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name='modelCard.vision'
                        render={({ field }) => (
                          <FormItem className='flex items-center space-y-0 space-x-2'>
                            <FormControl>
                              <Checkbox checked={field.value || false} onCheckedChange={field.onChange} />
                            </FormControl>
                            <FormLabel className='font-normal'>{t('models.modelCard.vision')}</FormLabel>
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name='modelCard.temperature'
                        render={({ field }) => (
                          <FormItem className='flex items-center space-y-0 space-x-2'>
                            <FormControl>
                              <Checkbox checked={field.value || false} onCheckedChange={field.onChange} />
                            </FormControl>
                            <FormLabel className='font-normal'>{t('models.modelCard.temperature')}</FormLabel>
                          </FormItem>
                        )}
                      />
                    </div>
                  </div>

                  <div className='space-y-2'>
                    <FormLabel>{t('models.modelCard.reasoning')}</FormLabel>
                    <div className='grid grid-cols-2 gap-2'>
                      <FormField
                        control={form.control}
                        name='modelCard.reasoning.supported'
                        render={({ field }) => (
                          <FormItem className='flex items-center space-y-0 space-x-2'>
                            <FormControl>
                              <Checkbox checked={field.value || false} onCheckedChange={field.onChange} />
                            </FormControl>
                            <FormLabel className='font-normal'>{t('models.modelCard.reasoningSupported')}</FormLabel>
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name='modelCard.reasoning.default'
                        render={({ field }) => (
                          <FormItem className='flex items-center space-y-0 space-x-2'>
                            <FormControl>
                              <Checkbox checked={field.value || false} onCheckedChange={field.onChange} />
                            </FormControl>
                            <FormLabel className='font-normal'>{t('models.modelCard.reasoningDefault')}</FormLabel>
                          </FormItem>
                        )}
                      />
                    </div>
                  </div>

                  <div className='space-y-2'>
                    <FormLabel>{t('models.modelCard.modalities')}</FormLabel>
                    <div className='grid grid-cols-2 gap-4'>
                      <FormField
                        control={form.control}
                        name='modelCard.modalities.input'
                        render={({ field }) => {
                          const modalityOptions = ['text', 'image', 'audio', 'video'];
                          return (
                            <FormItem>
                              <FormLabel className='text-xs'>{t('models.modelCard.input')}</FormLabel>
                              <div className='space-y-2'>
                                {modalityOptions.map((modality) => (
                                  <FormItem key={modality} className='flex items-center space-y-0 space-x-2'>
                                    <FormControl>
                                      <Checkbox
                                        checked={field.value?.includes(modality) || false}
                                        onCheckedChange={(checked) => {
                                          const current = field.value || [];
                                          if (checked) {
                                            field.onChange([...current, modality]);
                                          } else {
                                            field.onChange(current.filter((v) => v !== modality));
                                          }
                                        }}
                                      />
                                    </FormControl>
                                    <FormLabel className='font-normal'>{t(`models.modelCard.${modality}`)}</FormLabel>
                                  </FormItem>
                                ))}
                              </div>
                              <FormMessage />
                            </FormItem>
                          );
                        }}
                      />
                      <FormField
                        control={form.control}
                        name='modelCard.modalities.output'
                        render={({ field }) => {
                          const modalityOptions = ['text', 'image', 'audio', 'video'];
                          return (
                            <FormItem>
                              <FormLabel className='text-xs'>{t('models.modelCard.output')}</FormLabel>
                              <div className='space-y-2'>
                                {modalityOptions.map((modality) => (
                                  <FormItem key={modality} className='flex items-center space-y-0 space-x-2'>
                                    <FormControl>
                                      <Checkbox
                                        checked={field.value?.includes(modality) || false}
                                        onCheckedChange={(checked) => {
                                          const current = field.value || [];
                                          if (checked) {
                                            field.onChange([...current, modality]);
                                          } else {
                                            field.onChange(current.filter((v) => v !== modality));
                                          }
                                        }}
                                      />
                                    </FormControl>
                                    <FormLabel className='font-normal'>{t(`models.modelCard.${modality}`)}</FormLabel>
                                  </FormItem>
                                ))}
                              </div>
                              <FormMessage />
                            </FormItem>
                          );
                        }}
                      />
                    </div>
                  </div>

                  <div className='space-y-2'>
                    <FormLabel>{t('models.modelCard.cost')} ($/M tokens)</FormLabel>
                    <p className='text-xs text-muted-foreground'>{t('models.modelCard.costHint')}</p>
                    <div className='grid grid-cols-2 gap-2'>
                      <FormField
                        control={form.control}
                        name='modelCard.cost.input'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel className='text-xs'>{t('models.modelCard.input')}</FormLabel>
                            <FormControl>
                              <Input
                                type='number'
                                step='0.01'
                                {...field}
                                value={field.value ?? ''}
                                onChange={(e) => field.onChange(e.target.value ? Number(e.target.value) : undefined)}
                                placeholder='0'
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name='modelCard.cost.output'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel className='text-xs'>{t('models.modelCard.output')}</FormLabel>
                            <FormControl>
                              <Input
                                type='number'
                                step='0.01'
                                {...field}
                                value={field.value ?? ''}
                                onChange={(e) => field.onChange(e.target.value ? Number(e.target.value) : undefined)}
                                placeholder='0'
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name='modelCard.cost.cacheRead'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel className='text-xs'>{t('models.modelCard.cacheRead')}</FormLabel>
                            <FormControl>
                              <Input
                                type='number'
                                step='0.01'
                                {...field}
                                value={field.value ?? ''}
                                onChange={(e) => field.onChange(e.target.value ? Number(e.target.value) : undefined)}
                                placeholder='0'
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name='modelCard.cost.cacheWrite'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel className='text-xs'>{t('models.modelCard.cacheWrite')}</FormLabel>
                            <FormControl>
                              <Input
                                type='number'
                                step='0.01'
                                {...field}
                                value={field.value ?? ''}
                                onChange={(e) => field.onChange(e.target.value ? Number(e.target.value) : undefined)}
                                placeholder='0'
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </div>
                  </div>

                  <div className='space-y-2'>
                    <FormLabel>{t('models.modelCard.limit')}</FormLabel>
                    <div className='grid grid-cols-2 gap-2'>
                      <FormField
                        control={form.control}
                        name='modelCard.limit.context'
                        render={({ field }) => {
                          return (
                            <FormItem>
                              <FormLabel className='text-xs'>{t('models.modelCard.context')}</FormLabel>
                              <FormControl>
                                <Input
                                  type='number'
                                  {...field}
                                  value={field.value || ''}
                                  onChange={(e) => field.onChange(e.target.value ? Number(e.target.value) : undefined)}
                                  placeholder='128000'
                                />
                              </FormControl>
                              {field.value && (
                                <p className='text-muted-foreground text-xs'>
                                  {t('models.modelCard.context')}: {formatNumber(field.value)}
                                </p>
                              )}
                              <FormMessage />
                            </FormItem>
                          );
                        }}
                      />
                      <FormField
                        control={form.control}
                        name='modelCard.limit.output'
                        render={({ field }) => {
                          return (
                            <FormItem>
                              <FormLabel className='text-xs'>{t('models.modelCard.output')}</FormLabel>
                              <FormControl>
                                <Input
                                  type='number'
                                  {...field}
                                  value={field.value || ''}
                                  onChange={(e) => field.onChange(e.target.value ? Number(e.target.value) : undefined)}
                                  placeholder='4096'
                                />
                              </FormControl>
                              {field.value && (
                                <p className='text-muted-foreground text-xs'>
                                  {t('models.modelCard.output')}: {formatNumber(field.value)}
                                </p>
                              )}
                              <FormMessage />
                            </FormItem>
                          );
                        }}
                      />
                    </div>
                  </div>

                  <div className='space-y-2'>
                    <FormLabel>{t('models.modelCard.dates')}</FormLabel>
                    <div className='grid grid-cols-3 gap-2'>
                      <FormField
                        control={form.control}
                        name='modelCard.knowledge'
                        render={({ field }) => (
                          <FormItem className='flex flex-col'>
                            <FormLabel className='text-xs'>{t('models.modelCard.knowledge')}</FormLabel>
                            <Popover>
                              <PopoverTrigger asChild>
                                <FormControl>
                                  <Button
                                    variant='outline'
                                    className={cn('w-full pl-3 text-left font-normal', !field.value && 'text-muted-foreground')}
                                  >
                                    {field.value ? (
                                      format(new Date(field.value.length === 7 ? `${field.value}-01` : field.value), 'yyyy-MM-dd')
                                    ) : (
                                      <span>Pick a date</span>
                                    )}
                                    <CalendarIcon className='ml-auto h-4 w-4 opacity-50' />
                                  </Button>
                                </FormControl>
                              </PopoverTrigger>
                              <PopoverContent className='w-auto p-0' align='start'>
                                <Calendar
                                  mode='single'
                                  selected={
                                    field.value ? new Date(field.value.length === 7 ? `${field.value}-01` : field.value) : undefined
                                  }
                                  onSelect={(date) => field.onChange(date ? format(date, 'yyyy-MM-dd') : undefined)}
                                  initialFocus
                                />
                              </PopoverContent>
                            </Popover>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name='modelCard.releaseDate'
                        render={({ field }) => (
                          <FormItem className='flex flex-col'>
                            <FormLabel className='text-xs'>{t('models.modelCard.releaseDate')}</FormLabel>
                            <Popover>
                              <PopoverTrigger asChild>
                                <FormControl>
                                  <Button
                                    variant='outline'
                                    className={cn('w-full pl-3 text-left font-normal', !field.value && 'text-muted-foreground')}
                                  >
                                    {field.value ? format(new Date(field.value), 'yyyy-MM-dd') : <span>Pick a date</span>}
                                    <CalendarIcon className='ml-auto h-4 w-4 opacity-50' />
                                  </Button>
                                </FormControl>
                              </PopoverTrigger>
                              <PopoverContent className='w-auto p-0' align='start'>
                                <Calendar
                                  mode='single'
                                  selected={field.value ? new Date(field.value) : undefined}
                                  onSelect={(date) => field.onChange(date ? format(date, 'yyyy-MM-dd') : undefined)}
                                  initialFocus
                                />
                              </PopoverContent>
                            </Popover>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name='modelCard.lastUpdated'
                        render={({ field }) => (
                          <FormItem className='flex flex-col'>
                            <FormLabel className='text-xs'>{t('models.modelCard.lastUpdated')}</FormLabel>
                            <Popover>
                              <PopoverTrigger asChild>
                                <FormControl>
                                  <Button
                                    variant='outline'
                                    className={cn('w-full pl-3 text-left font-normal', !field.value && 'text-muted-foreground')}
                                  >
                                    {field.value ? format(new Date(field.value), 'yyyy-MM-dd') : <span>Pick a date</span>}
                                    <CalendarIcon className='ml-auto h-4 w-4 opacity-50' />
                                  </Button>
                                </FormControl>
                              </PopoverTrigger>
                              <PopoverContent className='w-auto p-0' align='start'>
                                <Calendar
                                  mode='single'
                                  selected={field.value ? new Date(field.value) : undefined}
                                  onSelect={(date) => field.onChange(date ? format(date, 'yyyy-MM-dd') : undefined)}
                                  initialFocus
                                />
                              </PopoverContent>
                            </Popover>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </div>
                  </div>
                </div>
              </div>
            </div>

            {isEdit && (
              <div className='space-y-3 rounded-md border p-3'>
                <div className='flex items-center justify-between'><FormLabel>协议池</FormLabel><Button type='button' variant='outline' size='sm' onClick={() => setProtocolPools([...protocolPools, { format: 'openai', associations: [] }])}>新增协议池</Button></div>
                {protocolError && <p className='text-sm text-destructive'>{protocolError}</p>}
                {protocolPools.map((pool, index) => <div key={`${pool.format}-${index}`} className='space-y-2 rounded border p-2'>
                  <div className='flex gap-2'><Select value={pool.format} onValueChange={(format) => setProtocolPools(protocolPools.map((item, i) => i === index ? { ...item, format } : item))}><SelectTrigger><SelectValue placeholder='选择协议' /></SelectTrigger><SelectContent><SelectItem value='openai'>openai</SelectItem><SelectItem value='anthropic'>anthropic</SelectItem></SelectContent></Select><Button type='button' variant='ghost' onClick={() => setProtocolPools(protocolPools.filter((_, i) => i !== index))}>删除协议池</Button></div>
                  {pool.associations.map((association, targetIndex) => <div key={targetIndex} className='space-y-2 rounded bg-muted/30 p-2'>
                    <div className='flex gap-2'><Input placeholder='物理模型 ID' value={association.channelModel?.modelId || ''} onChange={(event) => setProtocolPools(protocolPools.map((item, i) => i === index ? { ...item, associations: item.associations.map((target, j) => j === targetIndex ? { ...target, channelModel: { ...target.channelModel!, modelId: event.target.value } } : target) } : item))} /><Input className='w-24' type='number' placeholder='优先级' value={association.priority ?? 0} onChange={(event) => setProtocolPools(protocolPools.map((item, i) => i === index ? { ...item, associations: item.associations.map((target, j) => j === targetIndex ? { ...target, priority: Number(event.target.value) } : target) } : item))} /></div>
                    <div className='flex gap-2'><Select value={String(association.channelModel?.channelId || '')} onValueChange={(value) => setProtocolPools(protocolPools.map((item, i) => i === index ? { ...item, associations: item.associations.map((target, j) => j === targetIndex ? { ...target, channelModel: { ...target.channelModel!, channelId: Number(value) } } : target) } : item))}><SelectTrigger><SelectValue placeholder='选择渠道' /></SelectTrigger><SelectContent>{channels.filter((channel) => channel.endpoints?.some((endpoint) => endpoint.apiFormat === pool.format) || channel.defaultEndpoints?.some((endpoint) => endpoint.apiFormat === pool.format)).map((channel) => <SelectItem key={channel.id} value={String(channel.id)}>{channel.name}</SelectItem>)}</SelectContent></Select><label className='flex items-center gap-2 text-sm'><Checkbox checked={!association.disabled} onCheckedChange={(checked) => setProtocolPools(protocolPools.map((item, i) => i === index ? { ...item, associations: item.associations.map((target, j) => j === targetIndex ? { ...target, disabled: !checked } : target) } : item))} />启用</label><Button type='button' variant='ghost' onClick={() => setProtocolPools(protocolPools.map((item, i) => i === index ? { ...item, associations: item.associations.filter((_, j) => j !== targetIndex) } : item))}>删除 target</Button></div>
                  </div>)}
                  <Button type='button' variant='outline' size='sm' onClick={() => setProtocolPools(protocolPools.map((item, i) => i === index ? { ...item, associations: [...item.associations, { type: 'channel_model' as const, priority: 0, disabled: false, channelModel: { channelId: 0, modelId: '' } }] } : item))}>新增 target</Button>
                </div>)}
              </div>
            )}

            <div className='flex flex-shrink-0 justify-end gap-2 border-t pt-4'>
              <Button type='button' variant='outline' onClick={handleClose}>
                {t('common.buttons.cancel')}
              </Button>
              <Button type='submit' disabled={createModel.isPending || updateModel.isPending}>
                {isEdit ? t('common.buttons.save') : t('common.buttons.create')}
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
