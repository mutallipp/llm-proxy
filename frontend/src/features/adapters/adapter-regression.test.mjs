import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const srcRoot = join(import.meta.dirname, '..', '..');

function read(relativePath) {
  return readFileSync(join(srcRoot, relativePath), 'utf8');
}

test('adapter model initialization keeps query-derived arrays stable', () => {
  const source = read('features/adapters/index.tsx');
  const dialogStart = source.indexOf('function AdapterDialog');
  const dialogEnd = source.indexOf('// ===================== 重命名 Adapter 弹窗 =====================');
  const dialog = source.slice(dialogStart, dialogEnd);

  assert.match(source, /const EMPTY_MODELS: Model\[\] = \[\];/);
  assert.equal((source.match(/const models = useMemo\(/g) ?? []).length, 3);
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
