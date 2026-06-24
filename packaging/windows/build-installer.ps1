param(
  [string]$AgentBinary = "..\..\agent\remote-agent.exe",
  [string]$OutFile = "RemoteAgentInstaller.exe",
  [string]$RelayUrl = "ws://localhost:8080/ws",
  [string]$EnrollmentCode = "",
  [string]$OrgId = "",
  [string]$DeviceGroup = "",
  [string]$TurnUrl = "",
  [string]$TurnUser = "agent",
  [string]$TurnPass = "changeme",
  [string]$AgentCertFile = "",
  [string]$AgentKeyFile = "",
  [string]$AgentCAFile = "",
  [string]$Makensis = "makensis"
)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptDir "..\..")
$installerScript = Join-Path $scriptDir "remote-agent.nsi"

if (-not [System.IO.Path]::IsPathRooted($AgentBinary)) {
  $AgentBinary = Join-Path $repoRoot $AgentBinary
}
if (-not (Test-Path $AgentBinary)) {
  throw "Agent binary not found: $AgentBinary"
}
if (-not [System.IO.Path]::IsPathRooted($OutFile)) {
  $OutFile = Join-Path $scriptDir $OutFile
}

& $Makensis `
  "/V2" `
  "/DAGENT_BINARY=$AgentBinary" `
  "/DOUTPUT_FILE=$OutFile" `
  "/DRELAY_URL=$RelayUrl" `
  "/DENROLLMENT_CODE=$EnrollmentCode" `
  "/DORG_ID=$OrgId" `
  "/DDEVICE_GROUP=$DeviceGroup" `
  "/DTURN_URL=$TurnUrl" `
  "/DTURN_USER=$TurnUser" `
  "/DTURN_PASS=$TurnPass" `
  "/DAGENT_CERT_FILE=$AgentCertFile" `
  "/DAGENT_KEY_FILE=$AgentKeyFile" `
  "/DAGENT_CA_FILE=$AgentCAFile" `
  $installerScript

if ($LASTEXITCODE -ne 0) {
  throw "makensis failed with exit code $LASTEXITCODE"
}
