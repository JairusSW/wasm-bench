param([Parameter(Mandatory=$true)][ValidatePattern('^[a-zA-Z0-9_-]+$')][string]$Bundle)
$ErrorActionPreference = 'Stop'
if (-not [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
  throw 'This qualification requires native Windows, not an emulated process.'
}
$recipeRoot = Split-Path -Parent $PSScriptRoot
if ([System.Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture -ne [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture) {
  throw 'Native process and OS architectures must match; emulated qualification is refused.'
}
$evidence = Join-Path $recipeRoot "runs/$Bundle"
if ((Test-Path -LiteralPath $evidence) -or (Get-Item -LiteralPath $evidence -Force -ErrorAction SilentlyContinue)) {
  throw "Preserving existing evidence: $evidence"
}
$controller = Join-Path $recipeRoot 'bin/wasmbench.exe'
foreach ($tool in @($controller, (Join-Path $recipeRoot 'bin/adapter-wazero.exe'), (Join-Path $recipeRoot 'adapters/wasmtime/target/release/wasm-analyze.exe'))) {
  if (-not (Test-Path -LiteralPath $tool -PathType Leaf)) { throw "Build native tools before collection: $tool" }
}
$node = (Get-Command node.exe -CommandType Application).Source
function Invoke-Checked([string]$Program, [string[]]$Arguments) {
  & $Program @Arguments
  if ($LASTEXITCODE -ne 0) { throw "Native command failed ($LASTEXITCODE): $Program $Arguments" }
}
New-Item -ItemType Directory -Path $evidence | Out-Null
Push-Location $recipeRoot
try {
  Invoke-Checked $controller @('doctor') | Out-File (Join-Path $evidence 'doctor.json') -Encoding utf8
  Invoke-Checked $controller @('check','--suite','core','--runtimes','wazero,wazero-interpreter,v8','--out',"$evidence/check")
  Invoke-Checked $controller @('run','--archive-tools=true','--suite','core','--runtimes','wazero,wazero-interpreter,v8','--scenarios','compile,instantiate,first-call','--profile','timing','--launches','2','--samples','2','--operations','2','--warmup','0','--out',"$evidence/run")
  Invoke-Checked $controller @('reproduce',"$evidence/run",'--out',"$evidence/replayed")
  Invoke-Checked $controller @('restore-tools','--run',"$evidence/run",'--out',"$evidence/tools") | Out-File (Join-Path $evidence 'restoration.json') -Encoding utf8
  $restoration = Get-Content -Raw (Join-Path $evidence 'restoration.json') | ConvertFrom-Json
  Invoke-Checked $restoration.runner @('run','--lock',$restoration.lock,'--out',"$evidence/archived")
  # Instrumented evidence stays in a distinct pass; Linux collectors stay unsupported.
  Invoke-Checked $controller @('run','--suite','core','--runtimes','wazero,wazero-interpreter,v8','--scenarios','compile,instantiate,first-call','--profile','memory','--phase-barriers','--launches','1','--samples','2','--operations','1','--warmup','0','--out',"$evidence/memory")
  foreach ($name in @('check','run','replayed','archived','memory')) {
    Invoke-Checked $controller @('verify','--run',"$evidence/$name")
  }
  Invoke-Checked $node @('recipes/verify-windows-lifecycle.mjs',$evidence)
  Invoke-Checked $controller @('report','--run',"$evidence/run",'--memory-run',"$evidence/memory",'--out',"$evidence/report")
  Invoke-Checked $controller @('verify-report','--dir',"$evidence/report")
  Invoke-Checked $controller @('verify-report','--dir',"$evidence/report",'--recorded-builder')
  Write-Output "Native Windows lifecycle/replay qualified: $evidence"
} finally { Pop-Location }
