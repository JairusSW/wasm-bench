import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';

function digest(file) {
  const hash = crypto.createHash('sha256'), buffer = Buffer.alloc(1024 * 1024);
  const fd = fs.openSync(file, 'r');
  try { let n; while ((n = fs.readSync(fd, buffer, 0, buffer.length, null))) hash.update(buffer.subarray(0, n)); }
  finally { fs.closeSync(fd); }
  return hash.digest('hex');
}

// Independent copy-on-write files, never hardlinks. Bundle bytes, paths, and
// permissions remain unchanged. No result, artifact, or checksum is removed.
export function compactToolArchives(roots, {apply = false, minimumBytes = 1024 * 1024, progress = () => {}} = {}) {
  const sizes = new Map();
  function walk(dir, inTools = false) {
    for (const entry of fs.readdirSync(dir, {withFileTypes: true})) {
      const file = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(file, inTools || entry.name === 'tools');
      else if (inTools && entry.isFile()) {
        const stat = fs.statSync(file);
        if (stat.size >= minimumBytes) {
          const group = sizes.get(stat.size) ?? [];
          group.push(file); sizes.set(stat.size, group);
        }
      }
    }
  }
  for (const root of roots) {
    if (!fs.lstatSync(root).isDirectory()) throw new Error(`Not a directory: ${root}`);
    walk(root);
  }
  let duplicates = 0, duplicateBytes = 0, scanned = 0;
  for (const [size, files] of sizes) {
    if (files.length < 2) continue;
    const hashes = new Map();
    for (const file of files) {
      const before = fs.statSync(file), hash = digest(file), source = hashes.get(hash);
      scanned++;
      if (!source) { hashes.set(hash, file); continue; }
      if (apply) {
        const temporary = fs.mkdtempSync(path.join(path.dirname(file), '.compact-'));
        const replacement = path.join(temporary, 'copy');
        try {
          // FORCE fails without clone support: never allocate a full fallback copy.
          if (process.platform === 'darwin') execFileSync('/bin/cp', ['-c', '-p', source, replacement]);
          else fs.copyFileSync(source, replacement, fs.constants.COPYFILE_EXCL | fs.constants.COPYFILE_FICLONE_FORCE);
          if (digest(replacement) !== hash) throw new Error(`Source changed: ${source}`);
          const current = fs.statSync(file);
          if (current.ino !== before.ino || current.size !== before.size || current.mtimeMs !== before.mtimeMs || digest(file) !== hash) throw new Error(`Destination changed: ${file}`);
          fs.chmodSync(replacement, before.mode & 0o777);
          fs.utimesSync(replacement, before.atime, before.mtime);
          fs.renameSync(replacement, file);
        } finally {
          if (fs.existsSync(replacement)) fs.unlinkSync(replacement);
          fs.rmdirSync(temporary);
        }
      }
      duplicates++; duplicateBytes += size;
      if (duplicates % 25 === 0) progress({scanned, duplicates, duplicateBytes});
    }
  }
  return {apply, scanned, duplicates, duplicateBytes, scope: 'identical large archived tools only; results preserved; savings depend on existing filesystem sharing'};
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  const args = process.argv.slice(2), apply = args.includes('--apply');
  const roots = args.filter(arg => arg !== '--apply');
  if (!roots.length || roots.some(root => !['runs', 'reports'].includes(root))) throw new Error('Usage: node recipes/compact-tool-archives.mjs [--apply] runs [reports]');
  console.log(JSON.stringify(compactToolArchives(roots, {apply, progress: data => console.error(JSON.stringify(data))})));
}
