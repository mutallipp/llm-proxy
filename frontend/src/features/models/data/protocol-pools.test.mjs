import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const dataDir = import.meta.dirname;
const srcRoot = join(dataDir, '..', '..', '..');

function read(relativePath) {
  return readFileSync(join(srcRoot, relativePath), 'utf8');
}

test('protocol pool filters channels by endpoint capability namespace', () => {
  const helper = read('features/models/data/protocol-pools.ts');
  const dialog = read('features/models/components/models-action-dialog.tsx');

  assert.ok(helper.includes("openai: ['openai/']"));
  assert.ok(helper.includes("anthropic: ['anthropic/']"));
  assert.match(dialog, /channels\.filter\(\(channel\) => channelSupportsProtocolPool\(channel, pool\.format\)\)/);
  assert.doesNotMatch(dialog, /endpoint\.apiFormat === pool\.format/);
});

test('protocol pool channel select keeps Relay GIDs while saving numeric channel IDs', () => {
  const helper = read('features/models/data/protocol-pools.ts');
  const dialog = read('features/models/components/models-action-dialog.tsx');

  assert.match(helper, /export function parseChannelIdFromSelectValue\(value: string\): number \| null/);
  assert.match(helper, /extractNumberID\(value\)/);
  assert.match(helper, /Number\.isSafeInteger\(channelId\)/);
  assert.match(helper, /export function channelIdToSelectValue\(\s*channelId: number \| null \| undefined/);
  assert.match(dialog, /value=\{channelIdToSelectValue\(association\.channelModel\?\.channelId, channels\)\}/);
  assert.match(dialog, /channelId: parseChannelIdFromSelectValue\(value\) \?\? 0/);
  assert.match(dialog, /<SelectItem key=\{channel\.id\} value=\{channel\.id\}>/);
  assert.doesNotMatch(dialog, /channelId: Number\(value\)/);
});
