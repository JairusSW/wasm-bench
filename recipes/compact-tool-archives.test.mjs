import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {compactToolArchives} from './compact-tool-archives.mjs';

test('compaction preserves results, checksums, modes and independent file contents', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'archive-compaction-'));
  try {
    fs.mkdirSync(path.join(root, 'tools'));
    const a = path.join(root, 'tools/a'), b = path.join(root, 'tools/b');
    fs.writeFileSync(a, 'identical'); fs.writeFileSync(b, 'identical');
    fs.chmodSync(b, 0o444);
    fs.writeFileSync(path.join(root, 'trial.json'), 'identical');
    fs.writeFileSync(path.join(root, 'checksums.json'), 'unchanged');
    const options = {minimumBytes: 1};
    assert.equal(compactToolArchives([root], options).duplicates, 1);
    // The actual machine must support cloning for --apply. Other platforms
    // still exercise the deterministic dry-run and safe failure behavior.
    if (process.platform === 'darwin') {
      assert.equal(compactToolArchives([root], {...options, apply: true}).duplicates, 1);
      assert.equal(fs.statSync(b).mode & 0o777, 0o444);
      assert.notEqual(fs.statSync(a).ino, fs.statSync(b).ino);
      fs.writeFileSync(a, 'changed');
      assert.equal(fs.readFileSync(b, 'utf8'), 'identical');
    }
    assert.equal(fs.readFileSync(path.join(root, 'trial.json'), 'utf8'), 'identical');
    assert.equal(fs.readFileSync(path.join(root, 'checksums.json'), 'utf8'), 'unchanged');
    assert.deepEqual(fs.readdirSync(path.join(root, 'tools')), ['a', 'b']);
  } finally { fs.rmSync(root, {recursive: true, force: true}); }
});
