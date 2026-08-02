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
