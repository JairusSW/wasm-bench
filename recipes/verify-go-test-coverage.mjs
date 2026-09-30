import {createInterface} from 'node:readline';
import {pathToFileURL} from 'node:url';
import path from 'node:path';

// A package PASS does not establish that opt-in tests executed. This gate checks
// Go's structured event stream, not human-readable PASS strings from guest logs.
export class GoTestCoverage {
  constructor(packageName, required) {
    if (!packageName || !required.length || new Set(required).size !== required.length ||
        required.some(name => !/^Test[A-Za-z0-9_]+$/.test(name))) {
      throw new Error('one package and unique top-level Test names are required');
    }
    this.packageName = packageName;
    this.required = required;
    this.states = new Map();
    this.packagePassed = false;
    this.packageStarted = false;
    this.failed = false;
    this.skipped = new Set();
    this.events = 0;
  }
  accept(event) {
    if (!event || typeof event !== 'object' || Array.isArray(event) ||
        typeof event.Action !== 'string' || typeof event.Package !== 'string') {
      throw new Error('invalid Go test JSON event');
    }
    this.events++;
    if (event.Action === 'fail' || event.Action === 'build-fail') this.failed = true;
    if (event.Package !== this.packageName) return;
    if (this.packagePassed) throw new Error('events after required package completion');
    const name = event.Test;
    if (name === undefined) {
      if (event.Action === 'start') this.packageStarted = true;
      if (event.Action === 'pass') this.packagePassed = true;
      if (event.Action === 'skip') this.failed = true;
      return;
    }
    if (typeof name !== 'string' || !name) throw new Error('invalid test identity');
    const top = name.split('/')[0];
    if (!this.required.includes(top)) return;
    if (event.Action === 'skip') this.skipped.add(name);
    if (name !== top) return;
    if (event.Action === 'run') {
      if (this.states.has(name)) throw new Error('duplicate required test execution');
      this.states.set(name, 'run');
    } else if (['pass','fail','skip'].includes(event.Action)) {
      if (this.states.get(name) !== 'run') throw new Error('required test terminal event without execution');
      this.states.set(name, event.Action);
    }
  }
  finish() {
    if (this.failed || !this.events || !this.packageStarted || !this.packagePassed || this.skipped.size ||
        this.required.some(name => this.states.get(name) !== 'pass')) {
      throw new Error('required test coverage incomplete, skipped, failed, or truncated');
    }
    return {version:'go-required-test-coverage-v1', package:this.packageName, passed:this.required, skipped:[]};
  }
}

export async function verifyStream(input, packageName, required) {
  const coverage = new GoTestCoverage(packageName, required);
  for await (const line of createInterface({input, crlfDelay:Infinity})) {
    if (line.length > 4*1024*1024) throw new Error('oversized Go test event');
    if (!line.trim()) continue;
    coverage.accept(JSON.parse(line));
  }
  return coverage.finish();
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  try {
    const args = process.argv.slice(2);
    if (args.length !== 4 || args[0] !== '--package' || args[2] !== '--require') {
      throw new Error('usage: go test -json ... | node recipes/verify-go-test-coverage.mjs --package PACKAGE --require TestOne,TestTwo');
    }
    console.log(JSON.stringify(await verifyStream(process.stdin, args[1], args[3].split(','))));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
