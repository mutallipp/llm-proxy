import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const dataDir = import.meta.dirname;
const srcRoot = join(dataDir, '..', '..', '..');

function read(relativePath) {
  return readFileSync(join(srcRoot, relativePath), 'utf8');
}

function loadProtocolPoolHelpers() {
  const source = read('features/models/data/protocol-pools.ts');
  const parseFunction = source.match(/export function parseChannelIdFromSelectValue\(value: string\): number \| null \{[\s\S]*?\n\}/)?.[0];
  const selectValueFunction = source.match(/export function channelIdToSelectValue\([\s\S]*?\): string \{[\s\S]*?\n\}/)?.[0];
  assert.ok(parseFunction);
  assert.ok(selectValueFunction);

  const executable = `${parseFunction.replace(/^export /, '').replace('(value: string): number | null', '(value)')}\n${selectValueFunction
    .replace(/^export /, '')
    .replace(/[\s\S]*?(?=\{)/, 'function channelIdToSelectValue(channelId, channels) ')}\nreturn { parseChannelIdFromSelectValue, channelIdToSelectValue };`;
  return new Function('extractNumberID', executable)((id) => id.slice(id.lastIndexOf('/') + 1));
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

test('protocol pool helpers parse and restore axonhub Relay GIDs at runtime', () => {
  const { parseChannelIdFromSelectValue, channelIdToSelectValue } = loadProtocolPoolHelpers();
  const relayGid = 'gid://axonhub/Channel/42';
  const channels = [{ id: relayGid }, { id: '7' }];

  assert.equal(parseChannelIdFromSelectValue(relayGid), 42);
  assert.equal(parseChannelIdFromSelectValue('7'), 7);
  assert.equal(parseChannelIdFromSelectValue('gid://llm-proxy/Channel/42'), null);
  assert.equal(channelIdToSelectValue(42, channels), relayGid);
  assert.equal(channelIdToSelectValue(7, channels), '7');
});
