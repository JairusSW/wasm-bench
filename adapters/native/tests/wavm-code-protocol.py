#!/usr/bin/env python3
"""Check a built WAVM adapter's code protocol without executing guest code."""
import hashlib
import json
import subprocess
import sys
import tempfile
from pathlib import Path

if len(sys.argv) != 2:
    raise SystemExit("usage: wavm-code-protocol.py /path/to/adapter-native")
with tempfile.TemporaryDirectory(prefix="wasmbench-wavm-code-") as directory:
    artifact = Path(directory) / "return42.wasm"
    artifact.write_bytes(bytes.fromhex(
        "0061736d010000000105016000017f030201000707010372756e00000a06010400412a0b"
    ))
    preparation = {
        "profile": "code", "artifact": str(artifact),
        "artifact_sha256": hashlib.sha256(artifact.read_bytes()).hexdigest(),
        "workload": {"abi": "core", "reset": "stateless", "export": "run", "args": [],
                     "oracle": {"kind": "exact_u64", "expected": ["42"]}},
    }
    requests = [
        {"method": "describe"}, {"method": "prepare", "prepare": preparation},
        {"method": "inspect"},
        {"method": "prepare", "prepare": {**preparation, "artifact_sha256": "0" * 64}},
        {"method": "inspect"}, {"method": "close"},
    ]
    envelopes = [{"version": 1, "id": i + 1, **r} for i, r in enumerate(requests)]
    result = subprocess.run([sys.argv[1]], input="".join(json.dumps(r) + "\n" for r in envelopes),
                            capture_output=True, text=True, check=True, timeout=60)
    responses = [json.loads(line) for line in result.stdout.splitlines()]
    assert len(responses) == len(envelopes), result.stdout
    assert responses[0]["description"]["capabilities"]["can_measure_native_code_size"] is True
    assert responses[1]["status"] == "ok", responses[1]
    size, export = responses[2]["diagnostics"]
    assert size["metric"] == "native.code_size" and size["status"] == "available", size
    assert isinstance(size["value"], int) and size["value"] > 0, size
    assert size["unit"] == "bytes" and "relocatable" in size["reason"], size
    assert export["status"] == "unavailable", export
    assert responses[3]["status"] == "error" and "digest" in responses[3]["reason"], responses[3]
    assert responses[4]["status"] == "error" and "prepare required" in responses[4]["reason"], responses[4]
    print(f"WAVM code protocol passed: {size['value']} executable-section bytes")
