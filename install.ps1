# Lerd installer for Windows - https://lerd.sh
# Usage:
#   Install:   powershell -c "irm https://lerd.sh/install.ps1 | iex"
#   Options:   & ([scriptblock]::Create((irm https://lerd.sh/install.ps1))) -Beta
#              & ([scriptblock]::Create((irm https://lerd.sh/install.ps1))) -Version 1.30.0
#   Local:     .\install.ps1 -Local .\build\lerd.exe
# Running it again updates lerd to the newest release.

param(
  [string]$Version = '',
  [switch]$Beta,
  [string]$Local = ''
)

# REPO is the GitHub owner/name release assets come from; LERD_REPO overrides it
# so a fork can be installed from without editing the script.
$LerdRepo = if ($env:LERD_REPO) { $env:LERD_REPO } else { 'lerd-env/lerd' }

function Write-Info($msg)    { Write-Host "  --> $msg" -ForegroundColor Cyan }
function Write-Success($msg) { Write-Host "  OK  $msg" -ForegroundColor Green }
function Write-Failure($msg) { Write-Host "  X   $msg" -ForegroundColor Red }

function Get-LerdArch {
  # A 32-bit PowerShell on a 64-bit Windows reports x86 here and the real
  # architecture in PROCESSOR_ARCHITEW6432.
  $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
  switch ($arch) {
    'AMD64' { return 'amd64' }
    'ARM64' { return 'arm64' }
    default { throw "Unsupported architecture: $arch. Lerd ships Windows builds for amd64 and arm64." }
  }
}

# The tag ends up in a download URL and a file name, so anything beyond the
# characters a release tag carries is refused, as lerd update does.
function Test-LerdVersion([string]$v) {
  return $v -match '^[A-Za-z0-9._-]+$'
}

function ConvertFrom-ReleaseLocation([string]$location) {
  if ($location -match '/releases/tag/v?([^/\s]+)\s*$') { return $Matches[1] }
  return ''
}

function ConvertFrom-ReleaseFeed([string]$feed) {
  if ($feed -match 'releases/tag/v([^"<\s]+)') { return $Matches[1] }
  return ''
}

# releases/latest redirects to the newest stable tag and needs no API token.
function Get-LatestVersion {
  $req = [System.Net.HttpWebRequest]::Create("https://github.com/$LerdRepo/releases/latest")
  $req.AllowAutoRedirect = $false
  $req.UserAgent = 'lerd-installer'
  $resp = $req.GetResponse()
  try { return ConvertFrom-ReleaseLocation $resp.Headers['Location'] } finally { $resp.Close() }
}

# The redirect skips prereleases, so the beta line is read off the atom feed,
# which lists the newest release first.
function Get-LatestPrerelease {
  $feed = Invoke-WebRequest -UseBasicParsing -UserAgent 'lerd-installer' "https://github.com/$LerdRepo/releases.atom"
  return ConvertFrom-ReleaseFeed $feed.Content
}

function Resolve-LerdVersion {
  if ($Version) { return $Version.TrimStart('v') }
  if ($Beta) { return Get-LatestPrerelease }
  return Get-LatestVersion
}

function Get-InstalledLerdVersion {
  $cmd = Get-Command lerd -ErrorAction SilentlyContinue
  if (-not $cmd) { return '' }
  $out = & $cmd.Source --version 2>$null | Out-String
  if ($out -match '(\d+\.\d+\.\d+[A-Za-z0-9.-]*)') { return $Matches[1] }
  return ''
}

function Get-ExpectedHash([string]$checksums, [string]$filename) {
  foreach ($line in $checksums -split "`r?`n") {
    $parts = $line.Trim() -split '\s+'
    if ($parts.Count -eq 2 -and $parts[1].TrimStart('*') -eq $filename) { return $parts[0].ToLowerInvariant() }
  }
  return ''
}

function Get-LerdArchiveName([string]$v, [string]$arch) {
  return "lerd_${v}_windows_${arch}.zip"
}

# Downloads the release zip, checks it against the release's checksums.txt and
# extracts it into $dest.
function Save-LerdRelease([string]$v, [string]$arch, [string]$dest) {
  $file = Get-LerdArchiveName $v $arch
  $base = "https://github.com/$LerdRepo/releases/download/v$v"
  $zip = Join-Path $dest $file

  Write-Info "Downloading lerd v$v ($arch) ..."
  try {
    Invoke-WebRequest -UseBasicParsing -UserAgent 'lerd-installer' "$base/$file" -OutFile $zip
  } catch {
    throw "Download failed: $base/$file`n$($_.Exception.Message)"
  }

  $sums = (Invoke-WebRequest -UseBasicParsing -UserAgent 'lerd-installer' "$base/checksums.txt").Content
  if ($sums -is [byte[]]) { $sums = [Text.Encoding]::UTF8.GetString($sums) }
  $want = Get-ExpectedHash $sums $file
  if (-not $want) { throw "checksums.txt for v$v has no entry for $file" }
  $got = (Get-FileHash -Algorithm SHA256 $zip).Hash.ToLowerInvariant()
  if ($got -ne $want) { throw "Checksum mismatch for ${file}: expected $want, got $got" }
  Write-Success 'Checksum verified'

  Expand-Archive -Path $zip -DestinationPath $dest -Force
  $exe = Join-Path $dest 'lerd.exe'
  if (-not (Test-Path $exe)) { throw "lerd.exe not found in $file" }
  return $exe
}

# A local lerd.exe, or a release zip, for testing a build before it ships.
function Get-LocalLerd([string]$path, [string]$dest) {
  if (-not (Test-Path $path)) { throw "File not found: $path" }
  if ($path -like '*.zip') {
    Expand-Archive -Path $path -DestinationPath $dest -Force
    $exe = Join-Path $dest 'lerd.exe'
    if (-not (Test-Path $exe)) { throw "lerd.exe not found in $path" }
    return $exe
  }
  return (Resolve-Path $path).Path
}

function Install-Lerd {
  Write-Host "`nInstalling Lerd`n"
  $tmp = Join-Path ([IO.Path]::GetTempPath()) ("lerd-install-" + [guid]::NewGuid().ToString('N'))
  New-Item -ItemType Directory -Path $tmp | Out-Null
  try {
    if ($Local) {
      $exe = Get-LocalLerd $Local $tmp
    } else {
      $arch = Get-LerdArch
      $v = Resolve-LerdVersion
      if (-not $v) { throw "No releases found at https://github.com/$LerdRepo/releases" }
      if (-not (Test-LerdVersion $v)) { throw "Refusing unsafe release version '$v'" }
      if ((Get-InstalledLerdVersion) -eq $v) {
        Write-Success "Lerd v$v is already installed and up to date"
        return
      }
      $exe = Save-LerdRelease $v $arch $tmp
    }

    # lerd install copies itself, and lerd-tray.exe beside it, into
    # %LOCALAPPDATA%\lerd\bin, puts that on the user PATH and sets up the rest.
    Write-Info "Running 'lerd install' to complete setup ..."
    Write-Host ''
    & $exe install
    # A first install may stop for a reboot before lerd is on the PATH, so the
    # way back names the copy lerd install already placed.
    if ($LASTEXITCODE -ne 0) {
      throw "lerd install did not finish. Once you have done what it asks, run:`n`n    & `"$env:LOCALAPPDATA\lerd\bin\lerd.exe`" install"
    }

    Write-Host ''
    Write-Success 'Open a new terminal so it picks up lerd on the PATH.'
    Write-Host ''
    Write-Host "  If lerd is useful to you, a GitHub star helps others find it:"
    Write-Host "     https://github.com/$LerdRepo"
  } finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
  }
}

# Errors are thrown and reported here rather than ending in exit, which would
# close the user's window under irm | iex.
function Invoke-LerdInstaller {
  $ErrorActionPreference = 'Stop'
  # Invoke-WebRequest's progress bar slows Windows PowerShell 5.1 downloads to a crawl.
  $ProgressPreference = 'SilentlyContinue'
  # Windows PowerShell 5.1 on older builds does not offer TLS 1.2 by default.
  [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
  try {
    Install-Lerd
  } catch {
    Write-Failure $_.Exception.Message
  }
}

# Dot-sourcing loads the functions for the tests without installing anything.
if ($MyInvocation.InvocationName -ne '.') { Invoke-LerdInstaller }
