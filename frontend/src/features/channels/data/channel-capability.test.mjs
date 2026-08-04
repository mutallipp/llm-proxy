import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const dataDir = import.meta.dirname;
const srcRoot = join(dataDir, '..', '..', '..');
const read = (relativePath) => readFileSync(join(srcRoot, relativePath), 'utf8');

function parseLocale(locale) {
  return JSON.parse(read(`locales/${locale}/channels.json`));
}

test('channel capability editor uses the protocol-pool whitelist and inherited exceptions', () => {
  const schema = read('features/channels/data/schema.ts');
  const dialog = read('features/channels/components/channels-capability-dialog.tsx');

  assert.match(schema, /protocolPoolFormats/);
  assert.match(schema, /saveChannelCapabilitiesInputSchema/);
  assert.match(dialog, /protocolPoolFormats\.map/);
  assert.match(dialog, /channelSupportsProtocolPool\(channel, protocol\)/);
  assert.match(dialog, /protocols: modelProtocols\[modelId\]/);
});

test('channel capability locales stay aligned for save results and endpoint review notices', () => {
  const en = parseLocale('en');
  const zh = parseLocale('zh-CN');
  for (const key of [
    'channels.capability.saved',
    'channels.capability.unmatched',
    'channels.capability.revoked.autoDisabled',
    'channels.capability.revoked.manualNotice',
    'channels.capability.bulkEnable.button',
  ]) {
    assert.equal(typeof en[key], 'string');
    assert.equal(typeof zh[key], 'string');
  }
});
