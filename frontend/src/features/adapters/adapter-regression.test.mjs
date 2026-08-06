import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const srcRoot = join(import.meta.dirname, '..', '..');

function read(relativePath) {
  return readFileSync(join(srcRoot, relativePath), 'utf8');
}

test('adapter page imports the navigation hook used by its dialogs', () => {
  const source = read('features/adapters/index.tsx');

  assert.match(source, /import \{ useNavigate \} from '@tanstack\/react-router';/);
});

test('adapter model initialization keeps query-derived arrays stable', () => {
  const source = read('features/adapters/index.tsx');
  const dialogStart = source.indexOf('function AdapterDialog');
  const dialogEnd = source.indexOf('// ===================== 重命名 Adapter 弹窗 =====================');
  const dialog = source.slice(dialogStart, dialogEnd);

  assert.match(source, /const EMPTY_MODELS: Model\[\] = \[\];/);
  // 列表页绑定详情区块已移除，当前仅两个弹窗需要派生模型数组。
  assert.equal((source.match(/const models = useMemo\(/g) ?? []).length, 2);
  assert.doesNotMatch(source, /const models = modelsData\?\.edges\.map/);

  const initializationEffectStart = dialog.indexOf('useEffect(() => {');
  const defaultModelEffectStart = dialog.indexOf('useEffect(() => {', initializationEffectStart + 1);
  const initializationEffect = dialog.slice(initializationEffectStart, defaultModelEffectStart);
  const defaultModelEffect = dialog.slice(defaultModelEffectStart);

  assert.match(initializationEffect, /setModelId\(''\)/);
  assert.match(initializationEffect, /\}, \[adapter, open\]\);/);
  assert.match(defaultModelEffect, /setModelId\(firstModelId\)/);
  assert.match(defaultModelEffect, /\}, \[firstModelId, modelId, open\]\);/);
});

test('adapter model select converts numeric and Model Relay IDs safely', () => {
  const source = read('features/adapters/index.tsx');
  const api = read('lib/adapterApi.ts');
  const parserStart = source.indexOf('const NUMERIC_MODEL_ID_PATTERN');
  const parserEnd = source.indexOf('\nfunction modelIdToSelectValue');
  const parserSource = source
    .slice(parserStart, parserEnd)
    .replaceAll('value: string', 'value')
    .replaceAll(': number | null', '');
  const parseModelId = new Function(
    'extractNumberIDAsNumber',
    `${parserSource}; return parseModelIdFromSelectValue;`
  )((value) => Number(value.slice(value.lastIndexOf('/') + 1)));
  const selectValueStart = source.indexOf('function modelIdToSelectValue');
  const selectValueEnd = source.indexOf('\nfunction findModelById');
  const selectValueSource = source
    .slice(selectValueStart, selectValueEnd)
    .replaceAll('modelId: number | null | undefined', 'modelId')
    .replaceAll('models: readonly Model[]', 'models')
    .replaceAll(': string', '');
  const modelIdToSelectValue = new Function(
    'parseModelIdFromSelectValue',
    `${selectValueSource}; return modelIdToSelectValue;`
  )(parseModelId);
  const models = [{ id: 'gid://axonhub/Model/1' }, { id: 'gid://axonhub/Model/2' }];

  assert.equal(parseModelId('1'), 1);
  assert.equal(parseModelId('gid://axonhub/Model/1'), 1);
  assert.equal(parseModelId('gid://axonhub/Model/9007199254740992'), null);
  assert.equal(parseModelId('gid://axonhub/Channel/1'), null);
  assert.equal(parseModelId('gid://axonhub/Model/0'), null);
  assert.equal(parseModelId('not-a-model-id'), null);
  assert.equal(modelIdToSelectValue(1, models), 'gid://axonhub/Model/1');
  assert.equal(modelIdToSelectValue(2, models), 'gid://axonhub/Model/2');
  assert.equal(modelIdToSelectValue(0, models), '');

  assert.match(source, /import \{ extractNumberIDAsNumber \} from '@\/lib\/utils';/);
  assert.match(source, /const selectedModelId = parseModelIdFromSelectValue\(modelId\);/);
  assert.match(source, /model_id: selectedModelId/);
  assert.match(source, /modelIdToSelectValue\(parseModelIdFromSelectValue\(models\[0\]\.id\), models\)/);
  assert.match(source, /findModelById\(models, binding\.model_id\)/);
  assert.doesNotMatch(source, /Number\(modelId\)/);
  assert.doesNotMatch(source, /Number\(item\.id\)/);
  assert.doesNotMatch(source, /Number\(model\.id\)/);
  assert.match(api, /model_id: number;/);
  assert.doesNotMatch(api, /model_group_id/);
});
