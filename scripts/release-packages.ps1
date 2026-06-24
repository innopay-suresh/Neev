param(
  [string]$AgentBinary = "$PSScriptRoot\..\agent\remote-agent.exe",
  [string]$OutputDir = "$PSScriptRoot\..\dist\packages",
  [string]$Version = "1.0.0",
  [string]$RelayUrl = "ws://localhost:8080/ws",
  [string]$EnrollmentCode = "",
  [string]$OrgId = "",
  [string]$DeviceGroup = "",
  [string]$TurnUrl = "",
  [string]$TurnUser = "agent",
  [string]$TurnPass = "changeme",
  [string]$Makensis = "makensis"
)

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
if (-not [System.IO.Path]::IsPathRooted($AgentBinary)) {
  $AgentBinary = Join-Path $repoRoot $AgentBinary
}
if (-not (Test-Path $AgentBinary)) {
  throw "Agent binary not found: $AgentBinary"
}

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$outputFile = Join-Path $OutputDir "RemoteAgentInstaller.exe"

& $Makensis `
  "/V2" `
  "/DAGENT_BINARY=$AgentBinary" `
  "/DOUTPUT_FILE=$outputFile" `
  "/DRELAY_URL=$RelayUrl" `
  "/DENROLLMENT_CODE=$EnrollmentCode" `
  "/DORG_ID=$OrgId" `
  "/DDEVICE_GROUP=$DeviceGroup" `
  "/DTURN_URL=$TurnUrl" `
  "/DTURN_USER=$TurnUser" `
  "/DTURN_PASS=$TurnPass" `
  "$repoRoot\packaging\windows\remote-agent.nsi"

if ($LASTEXITCODE -ne 0) {
  throw "makensis failed with exit code $LASTEXITCODE"
}
