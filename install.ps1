# Lerd installer for Windows — https://lerd.sh
# Usage, from a PowerShell window (no need to open it as administrator):
#   irm https://lerd.sh/install.ps1 | iex
#
# The console counterpart of Install-Lerd.exe (lerd-desktop's installer/), and it
# takes the same steps: it takes a Windows machine with nothing on it to a
# running lerd. The WSL features (and the restart they need), WSL itself, an
# Ubuntu distro with a Linux user, Podman, the Lerd desktop app, lerd's own
# install.sh inside the distro, then `lerd wsl:setup`, which wires lerd into
# Windows. Every step checks before it acts, so running it again after a failure
# resumes where it stopped. Keep the two in step.
#
# It runs as you, so the distro, the registry entries and the desktop app all
# belong to the signed-in user, and asks Windows for administrator rights only
# for the steps that need them: enabling the features, installing WSL, and
# putting the desktop app under Program Files.
#
# LERD_SETUP_LOCAL=<folder> names a Windows folder holding install.sh, lerd and
# lerd.exe, and optionally lerd-desktop-win32-<arch>.zip, to test a build before
# it is released.

# Everything runs inside one script block, so `irm | iex` leaves no variables or
# functions behind in the caller's session, and a failure returns to the prompt
# instead of closing the window the way `exit` would.
& {
  $ErrorActionPreference = 'Stop'
  $ProgressPreference = 'SilentlyContinue'
  [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

  $FallbackDistro = 'Ubuntu-26.04'
  $MinUbuntu = 2404
  $Distro = $FallbackDistro
  # lerd's installer asks sudo for its password, and the steps run detached from
  # the terminal, so the user gets passwordless sudo only while they run.
  $SudoersDropIn = '/etc/sudoers.d/zz-lerd-setup'
  $LxssKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Lxss'
  $RunKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
  $RunOnceKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\RunOnce'
  # The AppUserModelID lerd's own toasts use, shared by the desktop app.
  $ToastAppID = 'sh.lerd.Lerd'
  $Repo = 'lerd-env/lerd'
  # Where Install-Lerd.exe installs too: wsl:setup leaves lerd's agent to a desktop
  # app started from Program Files, which Windows Defender lets be.
  $AppDir = Join-Path $env:ProgramW6432 'Lerd'
  $DesktopDir = Join-Path $AppDir 'desktop'
  $DesktopExe = Join-Path $DesktopDir 'lerd-desktop.exe'
  $DesktopVersionFile = Join-Path $AppDir 'desktop.version'
  $Local = $env:LERD_SETUP_LOCAL
  $env:WSL_UTF8 = '1'
  # What the steps learn and the answers they act on. A table, so the functions
  # below change it in place instead of reaching for $script: scope, which `irm
  # | iex` would make the caller's own session.
  $S = @{ LinuxUser = ''; Password = $null; UserExists = $false; LerdInstalled = $false; DNSLocalhost = $false; ManageNode = $true; OptionsPending = $false }
  # Choices wait here until applied, so a run cut off after install.sh still applies them.
  $OptionsFile = Join-Path $env:LOCALAPPDATA 'lerd\setup-options.json'

  # ---- output ----------------------------------------------------------------

  function Info($s) { Write-Host "  --> $s" -ForegroundColor Cyan }
  function Ok($s) { Write-Host "  OK  $s" -ForegroundColor Green }
  function Warn($s) { Write-Host "  !   $s" -ForegroundColor Yellow }
  function Step($s) { Write-Host ''; Write-Host "==> $s" -ForegroundColor White }

  # ---- running commands ------------------------------------------------------

  # RealWSLExe is where the WSL package installs wsl.exe. The one in System32 is
  # a stub until then, and asked anything it opens its own prompt offering to
  # install or update WSL.
  $RealWSLExe = Join-Path $env:ProgramW6432 'WSL\wsl.exe'
  function WSLExe {
    if (Test-Path $RealWSLExe) { $RealWSLExe } else { Join-Path $env:SystemRoot 'System32\wsl.exe' }
  }

  # Lit makes $s a single-quoted PowerShell literal, for paths written into the
  # scripts Elevated runs: a profile like O'Brien would otherwise end the string.
  function Lit([string]$s) { "'" + ($s -replace "'", "''") + "'" }

  # Sh makes $s a single-quoted literal for the Linux shell, where '' is no escape.
  function Sh([string]$s) { "'" + ($s -replace "'", "'\''") + "'" }

  # Quote escapes $s as one argument for wsl.exe's CommandLineToArgvW parsing;
  # PowerShell 5.1 passes embedded quotes unescaped, so lines are built by hand.
  function Quote([string]$s) {
    $r = '"'; $n = 0
    foreach ($c in $s.ToCharArray()) {
      if ($c -eq '\') { $n++ }
      else {
        if ($c -eq '"') { $r += '\' * ($n + 1) }
        $n = 0
      }
      $r += $c
    }
    $r + ('\' * $n) + '"'
  }

  # Detached runs a script in a new session with no controlling terminal: the
  # console gives WSL a pty, where a question read from /dev/tty would wait, and
  # without one every question takes its default.
  function Detached([string]$shell, [string]$body) {
    "--exec setsid -w $shell " + (Quote ("{`n" + $body + "`n} </dev/null"))
  }

  # --exec rather than --, which would hand the script to the user's shell first.
  function AsRoot([string]$body) { "-d $Distro -u root " + (Detached 'sh -c' $body) }
  function AsUser([string]$body) { "-d $Distro -u $($S.LinuxUser) --cd ~ " + (Detached 'bash -lc' $body) }

  function NewProcess([string]$exe, [string]$params) {
    $p = New-Object System.Diagnostics.ProcessStartInfo $exe, $params
    $p.UseShellExecute = $false
    $p
  }

  # Run runs a step's command with its output straight in the console, and throws
  # naming the step when it fails.
  function Run([string]$status, [string]$exe, [string]$params) {
    $proc = [Diagnostics.Process]::Start((NewProcess $exe $params))
    $proc.WaitForExit()
    if ($proc.ExitCode -ne 0) { throw "$status failed (exit code $($proc.ExitCode))" }
  }

  # Capture runs a command quietly and returns its exit code and trimmed output
  # lines. Stray NULs are dropped, in case wsl.exe writes UTF-16 regardless.
  function Capture([string]$exe, [string]$params) {
    $si = NewProcess $exe $params
    $si.RedirectStandardOutput = $true
    $si.RedirectStandardError = $true
    $si.StandardOutputEncoding = [Text.Encoding]::UTF8
    try { $proc = [Diagnostics.Process]::Start($si) } catch { return @{ Code = -1; Lines = @() } }
    $err = $proc.StandardError.ReadToEndAsync()
    $out = $proc.StandardOutput.ReadToEnd()
    $proc.WaitForExit()
    $null = $err.Result
    $lines = @($out -split "`r?`n" | ForEach-Object { $_.Replace("`0", '').Trim() } | Where-Object { $_ })
    @{ Code = $proc.ExitCode; Lines = $lines }
  }

  # Elevated runs a PowerShell script as administrator, which makes Windows ask,
  # and shows what it printed. It returns the script's exit code.
  function Elevated([string]$body) {
    $log = [IO.Path]::GetTempFileName()
    $wrapped = "`$ProgressPreference = 'SilentlyContinue'; `$ErrorActionPreference = 'Stop'; " +
      "try { & { $body } *>&1 | Out-File -Encoding utf8 $(Lit $log); exit 0 } " +
      "catch { `$_ | Out-File -Append -Encoding utf8 $(Lit $log); exit 1 }"
    $enc = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($wrapped))
    try {
      $proc = Start-Process powershell.exe -Verb RunAs -Wait -PassThru -WindowStyle Hidden `
        -ArgumentList '-NoProfile', '-ExecutionPolicy', 'Bypass', '-EncodedCommand', $enc
    } catch {
      throw 'Windows did not grant administrator rights'
    }
    Get-Content $log -ErrorAction SilentlyContinue | Where-Object { $_.Trim() } | ForEach-Object { Write-Host "      $_" }
    Remove-Item $log -ErrorAction SilentlyContinue
    $proc.ExitCode
  }

  function IsAdmin {
    ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
      [Security.Principal.WindowsBuiltInRole]::Administrator)
  }

  # NativeArch reads the machine's own architecture, which an x64 PowerShell
  # emulated on Arm64 does not report as its own.
  function NativeArch {
    $a = (Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Environment').PROCESSOR_ARCHITECTURE
    if ($a -eq 'ARM64') { 'arm64' } else { 'x64' }
  }

  # ---- state checks ----------------------------------------------------------

  function WSLFeaturesEnabled {
    foreach ($f in 'Microsoft-Windows-Subsystem-Linux', 'VirtualMachinePlatform') {
      # Get-WindowsOptionalFeature needs administrator rights; dism's English
      # output does not survive translation, so the CIM class answers instead.
      $s = (Get-CimInstance Win32_OptionalFeature -Filter "Name = '$f'").InstallState
      if ($s -ne 1) { return $false }
    }
    $true
  }

  # WSLInstalled reports whether `wsl --version` ends its first line in an
  # a.b.c.d version. Its labels are translated, and the inbox stub never prints one.
  function WSLInstalled {
    if (-not (Test-Path $RealWSLExe)) { return $false }
    $r = Capture $RealWSLExe '--version'
    ($r.Code -eq 0) -and ($r.Lines.Count -gt 0) -and ($r.Lines[0] -match ':\s*\d+\.\d+\.\d+\.\d+$')
  }

  function DistroInstalled {
    if (-not (Test-Path $RealWSLExe)) { return $false }
    $r = Capture $RealWSLExe '-l -q'
    ($r.Code -eq 0) -and ($r.Lines -contains $Distro)
  }

  # NewestUbuntu returns the newest Ubuntu-NN.NN in a `wsl` list. Lerd needs
  # podman 4.5, which Ubuntu ships from 24.04 on, so anything older does not count.
  function NewestUbuntu([string[]]$lines) {
    $lines | ForEach-Object { if ($_ -match '^(Ubuntu-(\d\d)\.(\d\d))(\s|$)') { $Matches[1] } } |
      Where-Object { [int]($_ -replace '\D') -ge $MinUbuntu } |
      Sort-Object { [int]($_ -replace '\D') } | Select-Object -Last 1
  }

  # Wired reads the distro or user Windows already runs Lerd with, from lerd-wsl.conf.
  function Wired([string]$key) {
    $conf = Join-Path $env:LOCALAPPDATA 'lerd\bin\lerd-wsl.conf'
    if (Test-Path $conf) {
      Get-Content $conf | Where-Object { $_ -like "$key=*" } | ForEach-Object { $_.Substring($key.Length + 1).Trim() }
    }
  }

  # InstalledDistro is the distro to stay on: the one Lerd runs from, or else the
  # newest usable Ubuntu. Moving to another would leave the user's sites behind.
  function InstalledDistro {
    if (-not (Test-Path $RealWSLExe)) { return $null }
    $r = Capture $RealWSLExe '-l -q'
    if ($r.Code -ne 0) { return $null }
    $wired = Wired 'distro'
    if ($wired -and ($r.Lines -contains $wired)) { return $wired }
    NewestUbuntu $r.Lines
  }

  # OfferedUbuntu is empty on a WSL too old to list a usable Ubuntu. A failed
  # lookup gives the fallback, so the install reports the network error itself.
  function OfferedUbuntu {
    $r = Capture $RealWSLExe '--list --online'
    if ($r.Code -eq 0) { NewestUbuntu $r.Lines } else { $FallbackDistro }
  }

  # PickDistro stays on a distro already here: a newer one on offer would
  # otherwise get a second, empty distro beside it.
  function PickDistro {
    $d = InstalledDistro
    if (-not $d) { $d = OfferedUbuntu }
    if ($d) { $d } else { $FallbackDistro }
  }

  function WSLReady { (WSLInstalled) -and ((InstalledDistro) -or (OfferedUbuntu)) }

  # ExistingUser returns the user Lerd runs as here, or else the distro's uid 1000
  # user, the one this script creates.
  function ExistingUser {
    $wired = if ($Distro -eq (Wired 'distro')) { Wired 'user' }
    foreach ($u in @($wired, 1000)) {
      if (-not $u) { continue }
      $r = Capture (WSLExe) "-d $Distro -u root --exec id -nu $u"
      if (($r.Code -eq 0) -and ($r.Lines.Count -gt 0)) { return $r.Lines[0] }
    }
    ''
  }

  # LerdInstalledFor reports whether lerd is already in the user's ~/.local/bin,
  # in which case it keeps the choices it settled on and nothing is asked again.
  function LerdInstalledFor([string]$user) {
    (Capture (WSLExe) ("-d $Distro -u $user --cd ~ " + (Detached 'sh -c' 'test -x ~/.local/bin/lerd'))).Code -eq 0
  }

  # LinuxUsername derives a valid Linux login from a Windows user name, as
  # winshim.LinuxUsername does for lerd.exe's own setup.
  function LinuxUsername([string]$windows) {
    $u = ($windows.ToLowerInvariant() -replace '[^a-z0-9-]', '')
    if (-not $u) { $u = 'lerd' } elseif ($u[0] -notmatch '[a-z]') { $u = "u$u" }
    if ($u.Length -gt 32) { $u = $u.Substring(0, 32) }
    $u
  }

  $Reserved = 'root daemon bin sys sync games man lp mail news uucp proxy www-data backup list irc nobody systemd-network systemd-resolve messagebus syslog ubuntu' -split ' '

  # ---- questions -------------------------------------------------------------

  function AskYes([string]$q, [bool]$default) {
    $hint = if ($default) { '[Y/n]' } else { '[y/N]' }
    $a = (Read-Host "  ?   $q $hint").Trim()
    if (-not $a) { return $default }
    $a -match '^[Yy]'
  }

  function Plain([Security.SecureString]$s) {
    $b = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($s)
    try { [Runtime.InteropServices.Marshal]::PtrToStringBSTR($b) } finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($b) }
  }

  function AskUser {
    Write-Host ''
    Write-Host "  lerd runs inside an Ubuntu WSL distro, under its own Linux user."
    Write-Host "  Choose a name and a password for that user. sudo asks for this password inside Linux."
    $suggested = LinuxUsername $env:USERNAME
    while ($true) {
      $u = (Read-Host "  ?   Username [$suggested]").Trim()
      if (-not $u) { $u = $suggested }
      if ($u -cnotmatch '^[a-z][a-z0-9-]{0,31}$') {
        Warn 'The username must start with a lowercase letter and use only lowercase letters, digits and dashes, up to 32 characters.'
      } elseif ($Reserved -contains $u) {
        Warn "$u is already a system user in Ubuntu; choose another name."
      } else { break }
    }
    $S.LinuxUser = $u
    AskPassword
  }

  function AskPassword {
    while ($true) {
      $p1 = Plain (Read-Host '  ?   Password' -AsSecureString)
      if (-not $p1) { Warn 'Choose a password.'; continue }
      $p2 = Plain (Read-Host '  ?   Confirm password' -AsSecureString)
      if ($p1 -ceq $p2) { break }
      Warn 'The passwords do not match.'
    }
    $S.Password = $p1
  }

  # UserHasPassword reports whether $user has a usable password: an install
  # stopped between useradd and chpasswd leaves the user without one.
  function UserHasPassword([string]$user) {
    $r = Capture (WSLExe) "-d $Distro -u root --exec passwd -S $user"
    ($r.Code -eq 0) -and ($r.Lines.Count -gt 0) -and (($r.Lines[0] -split '\s+')[1] -eq 'P')
  }

  # AskOptions asks up front what lerd's installers would ask along the way,
  # where the steps have no terminal to show them.
  function AskOptions {
    Write-Host ''
    Write-Host '  Local domains:'
    Write-Host '    1) https://<name>.test with trusted certificates (recommended)'
    Write-Host '    2) http://<name>.localhost over plain HTTP, with no DNS or certificates'
    while ($true) {
      $a = (Read-Host '  ?   Choose [1]').Trim()
      if ($a -in '', '1') { $S.DNSLocalhost = $false; break }
      if ($a -eq '2') { $S.DNSLocalhost = $true; break }
    }
    Write-Host ''
    Write-Host '  lerd can install Node.js per project and put node, npm and npx shims on PATH.'
    $S.ManageNode = AskYes 'Let lerd manage Node.js versions?' $true
    Write-Host '  Both can be changed later, from the dashboard or the lerd command.'
  }

  # ---- steps -----------------------------------------------------------------

  # EnableFeatures turns on the features WSL 2 runs on. They only take effect
  # after a restart, so the script registers itself to run again at the next
  # sign-in and stops here.
  function EnableFeatures {
    Step 'Enabling the WSL features...'
    if ((Elevated 'Enable-WindowsOptionalFeature -Online -NoRestart -All -FeatureName Microsoft-Windows-Subsystem-Linux, VirtualMachinePlatform | Out-Null') -ne 0) {
      throw 'Enabling the WSL features failed'
    }
    if ($PSCommandPath) {
      $resume = "& '$($PSCommandPath.Replace("'", "''"))'"
    } else {
      $resume = 'irm https://lerd.sh/install.ps1 | iex'
    }
    if ($Local) { $resume = "`$env:LERD_SETUP_LOCAL = '$($Local.Replace("'", "''"))'; $resume" }
    New-Item -Path $RunOnceKey -Force | Out-Null
    Set-ItemProperty -Path $RunOnceKey -Name 'lerd-setup' -Value ("powershell.exe -NoProfile -ExecutionPolicy Bypass -NoExit -Command " + (Quote $resume))
    Write-Host ''
    Write-Host '  Windows needs a restart to finish enabling WSL. The installer carries on by itself after you sign back in.'
    if (AskYes 'Restart now?' $true) { Restart-Computer -Force }
  }

  # EnsureWSL installs WSL from Microsoft's GitHub release. The wsl.exe Windows
  # ships is a stub, and its Store-based install is unreliable.
  function EnsureWSL {
    if (WSLReady) { return }
    Step 'Installing WSL...'
    $arch = NativeArch
    $url = (Invoke-RestMethod https://api.github.com/repos/microsoft/WSL/releases/latest).assets |
      Where-Object { $_.name -like "*.$arch.msi" } | Select-Object -First 1 -ExpandProperty browser_download_url
    if (-not $url) { throw "No WSL $arch MSI found in the latest github.com/microsoft/WSL release" }
    # The elevated step downloads the package itself, into a folder only
    # administrators can change, and checks Microsoft signed it: a file in the
    # user's temp folder could be swapped while Windows asks for approval.
    Info "Downloading and installing $url"
    $code = Elevated ("`$d = New-Item -ItemType Directory -Path (Join-Path `$env:SystemRoot ('Temp\lerd-wsl-' + [guid]::NewGuid())); " +
      "`$m = Join-Path `$d 'wsl.msi'; Invoke-WebRequest -UseBasicParsing $(Lit $url) -OutFile `$m; " +
      "`$sig = Get-AuthenticodeSignature `$m; " +
      "if (`$sig.Status -ne 'Valid' -or `$sig.SignerCertificate.Subject -notmatch 'O=Microsoft Corporation') { throw 'The WSL package is not signed by Microsoft' }; " +
      "`$p = Start-Process msiexec.exe -Wait -PassThru -ArgumentList '/i', ('`"' + `$m + '`"'), '/qn', '/norestart'; " +
      "Remove-Item `$d -Recurse -Force -ErrorAction SilentlyContinue; " +
      "if (`$p.ExitCode -notin 0, 3010) { throw ""msiexec exited with `$(`$p.ExitCode)"" }")
    if ($code -ne 0) { throw 'Installing WSL failed' }
    # msiexec returns before Windows Installer finishes registering WSL, and the
    # stub asked in that window offers to install WSL all over again.
    for ($i = 0; $i -lt 60; $i++) {
      if (WSLReady) { return }
      Start-Sleep 2
    }
    throw 'WSL was installed but wsl.exe still does not report it'
  }

  function EnsureDistro {
    if (DistroInstalled) { return }
    Step "Installing $Distro..."
    Run "Installing $Distro" (WSLExe) "--install -d $Distro --no-launch --web-download"
  }

  # MarkProvisioned makes uid 1000 the distro's default user and tells WSL its
  # first-run setup is done, so the distro never asks for a user of its own.
  function MarkProvisioned([int]$uid) {
    Get-ChildItem $LxssKey -ErrorAction SilentlyContinue | ForEach-Object {
      if ((Get-ItemProperty $_.PSPath).DistributionName -eq $Distro) {
        Set-ItemProperty $_.PSPath -Name DefaultUid -Value $uid -Type DWord
        Set-ItemProperty $_.PSPath -Name RunOOBE -Value 0 -Type DWord
      }
    }
  }

  # EnsureUser gives the distro a default sudo user and systemd. The password
  # reaches chpasswd through WSLENV, never a command line or the disk, and
  # --terminate restarts the distro for the wsl.conf settings.
  function EnsureUser {
    $status = "Creating the Linux user $($S.LinuxUser)"
    Step "$status..."
    if (-not $S.UserExists) {
      Run $status (WSLExe) (AsRoot "useradd -m -s /bin/bash -G sudo $($S.LinuxUser)")
      $S.UserExists = $true
    }
    if ($S.Password) {
      $wslenv = $env:WSLENV
      $env:LERD_SETUP_PW = $S.Password
      $env:WSLENV = if ($wslenv) { "${wslenv}:LERD_SETUP_PW" } else { 'LERD_SETUP_PW' }
      try {
        Run $status (WSLExe) (AsRoot "printf '%s:%s\n' $($S.LinuxUser) `"`$LERD_SETUP_PW`" | chpasswd")
      } finally {
        Remove-Item Env:LERD_SETUP_PW -ErrorAction SilentlyContinue
        $env:WSLENV = $wslenv
        $S.Password = $null
      }
    }
    Run $status (WSLExe) (AsRoot ("f=/etc/wsl.conf; touch `$f`n" +
        "grep -q '^systemd=true' `$f || printf '\n[boot]\nsystemd=true\n' >> `$f`n" +
        "grep -q '^default=' `$f || printf '\n[user]\ndefault=$($S.LinuxUser)\n' >> `$f"))
    $uid = (Capture (WSLExe) "-d $Distro -u root --exec id -u $($S.LinuxUser)").Lines | Select-Object -First 1
    if (-not $uid) { $uid = 1000 }
    Run $status (WSLExe) "--terminate $Distro"
    MarkProvisioned $uid
  }

  # InstallPackages installs the packages lerd's installer checks for, and turns
  # on linger, because with no terminal its offer to do both reads as "no" and it
  # stops without Podman. Keep the list in step with install.sh and the WSL guide.
  function InstallPackages {
    Step 'Installing Podman and the packages lerd needs...'
    Run 'Installing the packages' (WSLExe) (AsRoot ("export DEBIAN_FRONTEND=noninteractive`n" +
        '{ ! command -v apt-get >/dev/null || { apt-get update -q && apt-get install -y -q podman uidmap fuse-overlayfs slirp4netns curl git unzip libnss3-tools; }; } && ' +
        "loginctl enable-linger $($S.LinuxUser)"))
  }

  # ---- desktop app -----------------------------------------------------------

  # The desktop app holds lerd's tray and notifications on Windows and keeps its
  # agent running. It has no installer of its own, so it is downloaded from the
  # latest lerd-desktop release and registered here.

  # SetShortcutAppID stamps the app id on a shortcut, which ties it to the app's
  # own windows and notifications. WScript.Shell cannot set it.
  function SetShortcutAppID([string]$lnk, [string]$id) {
    if (-not ('LerdSetup.Lnk' -as [type])) {
      Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
using System.Runtime.InteropServices.ComTypes;
namespace LerdSetup {
  [StructLayout(LayoutKind.Sequential, Pack = 4)]
  public struct PropertyKey { public Guid fmtid; public uint pid; }
  [StructLayout(LayoutKind.Explicit, Size = 16)]
  public struct PropVariant { [FieldOffset(0)] public ushort vt; [FieldOffset(8)] public IntPtr p; }
  [ComImport, Guid("886D8EEB-8CF2-4446-8D02-CDBA1DBDCF99"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
  interface IPropertyStore {
    void GetCount(out uint c);
    void GetAt(uint i, out PropertyKey k);
    void GetValue(ref PropertyKey k, out PropVariant v);
    void SetValue(ref PropertyKey k, ref PropVariant v);
    void Commit();
  }
  [ComImport, Guid("00021401-0000-0000-C000-000000000046")]
  class CShellLink {}
  public static class Lnk {
    public static void SetAppID(string path, string id) {
      object link = new CShellLink();
      IPersistFile file = (IPersistFile)link;
      file.Load(path, 2);
      PropertyKey key = new PropertyKey { fmtid = new Guid("9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3"), pid = 5 };
      PropVariant v = new PropVariant { vt = 31, p = Marshal.StringToCoTaskMemUni(id) };
      try {
        IPropertyStore store = (IPropertyStore)link;
        store.SetValue(ref key, ref v);
        store.Commit();
        file.Save(path, true);
      } finally {
        Marshal.FreeCoTaskMem(v.p);
      }
    }
  }
}
'@
    }
    [LerdSetup.Lnk]::SetAppID($lnk, $id)
  }

  # RegisterDesktopApp registers the app as lerd's: the app id its notifications
  # post under, the lerd:// scheme their clicks open, a Start menu shortcut, and
  # a hidden start at sign-in.
  function RegisterDesktopApp {
    $exe = "`"$DesktopExe`""
    $key = "HKCU:\Software\Classes\AppUserModelId\$ToastAppID"
    New-Item $key -Force | Out-Null
    Set-ItemProperty $key -Name DisplayName -Value 'Lerd'
    Set-ItemProperty $key -Name IconUri -Value (Join-Path $AppDir 'lerd.png')
    New-Item 'HKCU:\Software\Classes\lerd\DefaultIcon' -Force | Out-Null
    New-Item 'HKCU:\Software\Classes\lerd\shell\open\command' -Force | Out-Null
    Set-Item 'HKCU:\Software\Classes\lerd' -Value 'URL:Lerd'
    Set-ItemProperty 'HKCU:\Software\Classes\lerd' -Name 'URL Protocol' -Value ''
    Set-Item 'HKCU:\Software\Classes\lerd\DefaultIcon' -Value "$exe,0"
    Set-Item 'HKCU:\Software\Classes\lerd\shell\open\command' -Value "$exe `"%1`""
    Set-ItemProperty $RunKey -Name 'lerd-desktop' -Value "$exe --hidden"

    $lnk = Join-Path ([Environment]::GetFolderPath('Programs')) 'Lerd.lnk'
    $sc = (New-Object -ComObject WScript.Shell).CreateShortcut($lnk)
    $sc.TargetPath = $DesktopExe
    $sc.WorkingDirectory = $DesktopDir
    $sc.Save()
    try { SetShortcutAppID $lnk $ToastAppID } catch { Warn "Could not set the shortcut's app id: $_" }
  }

  # StopDesktopApp ends a running copy of the app, which holds its files open.
  function StopDesktopApp {
    Get-Process -Name lerd-desktop -ErrorAction SilentlyContinue |
      Where-Object { $_.Path -like "$DesktopDir\*" } | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep 1
  }

  # InstallDesktopApp unpacks the app under Program Files before wsl:setup, which
  # then leaves the agent to it. LERD_SETUP_LOCAL's lerd-desktop-win32-<arch>.zip,
  # when there, is used instead and unpacked on every run.
  function InstallDesktopApp {
    Step 'Installing the Lerd desktop app...'
    $arch = NativeArch
    $zipName = "lerd-desktop-win32-$arch.zip"
    $tag = ''
    $zipUrl = ''
    if ($Local -and (Test-Path (Join-Path $Local $zipName))) {
      $zip = Join-Path $Local $zipName
      Info "Using $zip"
    } else {
      $r = Invoke-RestMethod https://api.github.com/repos/lerd-env/lerd-desktop/releases/latest
      $a = $r.assets | Where-Object { $_.name -eq $zipName } | Select-Object -First 1
      if (-not $a) { throw "No $zipName in the latest github.com/lerd-env/lerd-desktop release" }
      $tag = $r.tag_name
      if ((Test-Path $DesktopExe) -and (Test-Path $DesktopVersionFile) -and
        ((Get-Content $DesktopVersionFile -Raw).Trim() -eq $tag)) {
        Info "The desktop app is already at $tag"
        RegisterDesktopApp
        return
      }
      $zipUrl = $a.browser_download_url
      Info "Downloading $zipUrl"
    }
    # The logo Windows shows on lerd's notifications.
    $pngUrl = "https://raw.githubusercontent.com/$Repo/main/internal/ui/icons/icon-512.png"

    StopDesktopApp
    Info 'Unpacking the desktop app'
    # Windows' own tar unpacks the zip, its complaints shown since a half-unpacked
    # app opens black. Errors Continue around it, or PowerShell 5.1 turns its
    # first stderr line into a terminating error before the exit code is read.
    $tar = Join-Path $env:SystemRoot 'System32\tar.exe'
    # A released app is downloaded by the elevated step itself, so what lands
    # in Program Files is never a file the user's other programs could swap.
    $getZip = if ($zipUrl) {
      "`$t = New-Item -ItemType Directory -Path (Join-Path `$env:SystemRoot ('Temp\lerd-desktop-' + [guid]::NewGuid())); " +
      "`$z = Join-Path `$t 'app.zip'; Invoke-WebRequest -UseBasicParsing $(Lit $zipUrl) -OutFile `$z; "
    } else { "`$z = $(Lit $zip); " }
    $code = Elevated ("`$d = $(Lit $DesktopDir); " + $getZip +
      "if (Test-Path `$d) { Remove-Item `$d -Recurse -Force }; " +
      "Remove-Item $(Lit $DesktopVersionFile) -ErrorAction SilentlyContinue; " +
      "New-Item -ItemType Directory -Force `$d | Out-Null; " +
      "Invoke-WebRequest -UseBasicParsing $(Lit $pngUrl) -OutFile $(Lit (Join-Path $AppDir 'lerd.png')); " +
      "`$ErrorActionPreference = 'Continue'; & $(Lit $tar) -xf `$z -C `$d 2>&1 | ForEach-Object { ""`$_"" }; " +
      "if (`$LASTEXITCODE -ne 0) { throw ""tar exited with `$LASTEXITCODE"" }; " +
      "if (`$t) { Remove-Item `$t -Recurse -Force -ErrorAction SilentlyContinue }; " +
      "if ($(Lit $tag)) { Set-Content -Path $(Lit $DesktopVersionFile) -Value $(Lit $tag) -NoNewline }")
    if (($code -ne 0) -or -not (Test-Path $DesktopExe)) { throw 'Installing the Lerd desktop app failed' }
    RegisterDesktopApp
  }

  # StartDesktopApp starts the app as the signed-in user, never with
  # administrator rights, even from an elevated window.
  function StartDesktopApp([string]$arguments) {
    if (IsAdmin) {
      Start-Process "$env:SystemRoot\System32\runas.exe" -WindowStyle Hidden -ArgumentList '/trustlevel:0x20000', (Quote "`"$DesktopExe`" $arguments")
    } elseif ($arguments) {
      Start-Process $DesktopExe -ArgumentList $arguments
    } else {
      Start-Process $DesktopExe
    }
  }

  # ---- lerd ------------------------------------------------------------------

  function GrantSudo {
    $r = Capture (WSLExe) (AsRoot "printf '%s ALL=(ALL) NOPASSWD:ALL\n' $($S.LinuxUser) > $SudoersDropIn && chmod 0440 $SudoersDropIn")
    if ($r.Code -ne 0) { throw 'sudo failed (exit code 1)' }
  }

  function RemoveSudoers {
    $null = Capture (WSLExe) (AsRoot "rm -f $SudoersDropIn")
  }

  # ApplyOptions turns answers that differ from the installers' defaults into lerd
  # commands, before wsl:setup reads the DNS mode to decide on the .test rule.
  function ApplyOptions {
    if (-not $S.OptionsPending) { return }
    if ($S.DNSLocalhost) {
      Step 'Applying your options...'
      Run 'Applying your options' (WSLExe) (AsUser '~/.local/bin/lerd dns:disable')
    }
    if (-not $S.ManageNode) {
      Step 'Applying your options...'
      Run 'Applying your options' (WSLExe) (AsUser '~/.local/bin/lerd node:unmanage')
    }
    Remove-Item $OptionsFile -ErrorAction SilentlyContinue
  }

  # TrustCA imports lerd's root CA from here, so Windows' confirmation opens in
  # front rather than behind, as wsl:setup's hidden certutil's can. Declining is
  # not a failure: the sites work, with a browser warning.
  function TrustCA {
    if ($S.OptionsPending -and $S.DNSLocalhost) { return }
    $crt = Join-Path $env:TEMP 'lerd-rootCA.crt'
    # No CA means .localhost mode, which has no certificates to trust.
    $r = Capture (WSLExe) (AsUser "cp `"`$(~/.local/share/lerd/bin/mkcert -CAROOT)/rootCA.pem`" `"`$(wslpath $(Sh $crt))`"")
    if ($r.Code -ne 0) { return }
    try {
      $c = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 $crt
      if (Test-Path "Cert:\CurrentUser\Root\$($c.Thumbprint)") { return }
      Step "Trusting lerd's certificate in Windows..."
      Write-Host "  Windows will now ask whether to install lerd's certificate authority."
      Write-Host '  Choose Yes, so your browsers trust your https://<name>.test sites.'
      $p = Start-Process "$env:SystemRoot\System32\certutil.exe" -Wait -PassThru -WindowStyle Hidden -ArgumentList '-addstore', '-user', '-f', 'Root', (Quote $crt)
      if ($p.ExitCode -ne 0) {
        Warn 'The certificate was not installed, so browsers will warn on your https sites. Run lerd wsl:setup inside WSL to be asked again.'
      }
    } finally {
      Remove-Item $crt -ErrorAction SilentlyContinue
    }
  }

  # InstallLerd runs lerd's own installer inside the distro, so Windows gets
  # what every Linux machine gets.
  function InstallLerd {
    if ($Local) {
      $body = "cd `"`$(wslpath $(Sh $Local))`" && bash install.sh --local ./lerd"
    } else {
      $body = 'curl -fsSL https://lerd.sh/install.sh | bash'
    }
    Step "Installing lerd inside $Distro..."
    Run 'Installing lerd' (WSLExe) (AsUser $body)
    ApplyOptions
    TrustCA
    Step 'Connecting lerd with Windows...'
    Run 'Connecting lerd with Windows' (WSLExe) (AsUser 'LERD_WSL_RESTART_HANDLED=1 ~/.local/bin/lerd wsl:setup')
    # The desktop app only receives lerd's notifications from the native sink;
    # the browser sink, lerd's default, sends them to dashboard pages.
    Step "Handing lerd's notifications to the desktop app..."
    Run 'Handing over notifications' (WSLExe) (AsUser '~/.local/bin/lerd notify target native')
  }

  # StartLerd restarts WSL so mirrored networking and the resolver handover take
  # effect, boots the distro again, and waits for the dashboard to answer.
  function StartLerd {
    Step 'Starting lerd...'
    Run 'Starting lerd' (WSLExe) '--shutdown'
    Run 'Starting lerd' (WSLExe) "-d $Distro --exec /bin/true"
    for ($i = 0; $i -lt 90; $i++) {
      try {
        Invoke-WebRequest -UseBasicParsing -TimeoutSec 2 http://127.0.0.1:7073/ | Out-Null
        return
      } catch {
        if ($_.Exception.Response) { return }
        Start-Sleep 2
      }
    }
    throw 'The dashboard did not answer on 127.0.0.1:7073 within 3 minutes'
  }

  # ---- main ------------------------------------------------------------------

  Write-Host ''
  Write-Host 'Lerd installer for Windows' -ForegroundColor White

  if ([Environment]::OSVersion.Version.Build -lt 19041) {
    Warn 'WSL 2 needs Windows 10 2004 (build 19041) or newer.'
    return
  }
  if ([Environment]::OSVersion.Version.Build -lt 22621) {
    Warn 'Mirrored networking needs Windows 11 22H2 or newer. Lerd will install, but .test sites may not open from Windows.'
  }
  Remove-ItemProperty -Path $RunOnceKey -Name 'lerd-setup' -ErrorAction SilentlyContinue

  $Distro = PickDistro

  # The user is asked for after the restart, so it is not typed twice.
  if (-not (WSLFeaturesEnabled)) {
    try { EnableFeatures } catch { Warn $_.Exception.Message; Warn 'Run the installer again to retry.' }
    return
  }

  if ((WSLInstalled) -and (DistroInstalled)) { $S.LinuxUser = ExistingUser }
  $S.UserExists = [bool]$S.LinuxUser
  $S.LerdInstalled = $S.UserExists -and (LerdInstalledFor $S.LinuxUser)

  if (-not $S.UserExists) {
    AskUser
  } elseif (-not (UserHasPassword $S.LinuxUser)) {
    Info "The Linux user $($S.LinuxUser) has no password yet; choose one, sudo asks for it inside Linux."
    AskPassword
  } else {
    Info "Using the existing Linux user $($S.LinuxUser)"
  }
  if (Test-Path $OptionsFile) {
    $o = Get-Content $OptionsFile -Raw | ConvertFrom-Json
    $S.DNSLocalhost = [bool]$o.DNSLocalhost
    $S.ManageNode = [bool]$o.ManageNode
    $S.OptionsPending = $true
  } elseif (-not $S.LerdInstalled) {
    AskOptions
    $null = New-Item -ItemType Directory -Force (Split-Path $OptionsFile)
    @{ DNSLocalhost = $S.DNSLocalhost; ManageNode = $S.ManageNode } | ConvertTo-Json | Set-Content $OptionsFile
    $S.OptionsPending = $true
  }

  try {
    EnsureWSL
    $Distro = PickDistro
    EnsureDistro
    EnsureUser
    InstallPackages
    InstallDesktopApp
    GrantSudo
    try { InstallLerd } finally { RemoveSudoers }
    StartLerd
    StartDesktopApp '--hidden'
  } catch {
    Write-Host ''
    Write-Host "  The installer could not finish installing lerd: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host '  Run it again to resume from this step.'
    return
  }

  Write-Host ''
  Ok "lerd is running, and its icon is in the tray. Open a new terminal and the lerd command works from Windows too."
  Write-Host "      Your sites live inside Linux, at \\wsl.localhost\$Distro\home\$($S.LinuxUser)."
  Write-Host ''
  Write-Host "  *  If lerd is useful to you, a GitHub star helps others find it:" -ForegroundColor Cyan
  Write-Host "     https://github.com/$Repo"
  Write-Host ''
  if (AskYes 'Open Lerd now?' $true) { StartDesktopApp '' }
}
