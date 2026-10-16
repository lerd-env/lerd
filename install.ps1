# Lerd installer for Windows — https://lerd.sh
# Usage, from a PowerShell window (no need to open it as administrator):
#   irm https://lerd.sh/install.ps1 | iex
#
# The console counterpart of lerd-setup.exe (lerd-env/windows-installer), and it
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

  $Distro = 'Ubuntu-24.04'
  # lerd's installer asks sudo for its password, and the steps run detached from
  # the terminal, so the user gets passwordless sudo only while they run.
  $SudoersDropIn = '/etc/sudoers.d/zz-lerd-setup'
  $LxssKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Lxss'
  $RunKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
  $RunOnceKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\RunOnce'
  # The AppUserModelID lerd's own toasts use, shared by the desktop app.
  $ToastAppID = 'sh.lerd.Lerd'
  $Repo = 'lerd-env/lerd'
  # Where lerd-setup.exe installs too: wsl:setup leaves lerd's agent to a desktop
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
  $S = @{ LinuxUser = ''; Password = $null; UserExists = $false; LerdInstalled = $false; DNSLocalhost = $false; ManageNode = $true }

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

  # Quote escapes $s as one argument for the CommandLineToArgvW rules wsl.exe
  # parses its command line with. Windows PowerShell 5.1 passes embedded quotes
  # to native commands unescaped, so command lines are built here by hand.
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

  # Detached wraps a script so nothing in it can wait on a terminal. The console
  # gives WSL a pty, where a question read from /dev/tty would wait for an answer
  # the steps never expect to be asked. In a new session with no controlling
  # terminal, /dev/tty fails to open and every question takes its default.
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
      "try { & { $body } *>&1 | Out-File -Encoding utf8 '$log'; exit 0 } " +
      "catch { `$_ | Out-File -Append -Encoding utf8 '$log'; exit 1 }"
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

  # ExistingUser returns the distro's uid 1000 user, the one this script creates.
  function ExistingUser {
    $r = Capture (WSLExe) "-d $Distro -u root --exec id -nu 1000"
    if (($r.Code -eq 0) -and ($r.Lines.Count -gt 0)) { $r.Lines[0] } else { '' }
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
    Write-Host "  lerd runs inside an Ubuntu 24.04 WSL distro, under its own Linux user."
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
    while ($true) {
      $p1 = Plain (Read-Host '  ?   Password' -AsSecureString)
      if (-not $p1) { Warn 'Choose a password.'; continue }
      $p2 = Plain (Read-Host '  ?   Confirm password' -AsSecureString)
      if ($p1 -ceq $p2) { break }
      Warn 'The passwords do not match.'
    }
    $S.LinuxUser = $u
    $S.Password = $p1
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
    if (WSLInstalled) { return }
    Step 'Installing WSL...'
    $arch = NativeArch
    $url = (Invoke-RestMethod https://api.github.com/repos/microsoft/WSL/releases/latest).assets |
      Where-Object { $_.name -like "*.$arch.msi" } | Select-Object -First 1 -ExpandProperty browser_download_url
    if (-not $url) { throw "No WSL $arch MSI found in the latest github.com/microsoft/WSL release" }
    $msi = Join-Path $env:TEMP 'lerd-wsl.msi'
    Info "Downloading $url"
    Invoke-WebRequest -UseBasicParsing $url -OutFile $msi
    Info 'Installing the WSL package (msiexec)'
    $code = Elevated "`$p = Start-Process msiexec.exe -Wait -PassThru -ArgumentList '/i', '`"$msi`"', '/qn', '/norestart'; if (`$p.ExitCode -notin 0, 3010) { throw ""msiexec exited with `$(`$p.ExitCode)"" }"
    Remove-Item $msi -ErrorAction SilentlyContinue
    if ($code -ne 0) { throw 'Installing WSL failed' }
    # msiexec returns before Windows Installer finishes registering WSL, and the
    # stub asked in that window offers to install WSL all over again.
    for ($i = 0; $i -lt 60; $i++) {
      if (WSLInstalled) { return }
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
  function MarkProvisioned {
    Get-ChildItem $LxssKey -ErrorAction SilentlyContinue | ForEach-Object {
      if ((Get-ItemProperty $_.PSPath).DistributionName -eq $Distro) {
        Set-ItemProperty $_.PSPath -Name DefaultUid -Value 1000 -Type DWord
        Set-ItemProperty $_.PSPath -Name RunOOBE -Value 0 -Type DWord
      }
    }
  }

  # EnsureUser gives the distro a sudo user and systemd, and makes that user the
  # default. The password reaches chpasswd through WSLENV, so it never shows up
  # on a command line or on disk. Both wsl.conf settings need the distro
  # restarted, which --terminate does.
  function EnsureUser {
    $status = "Creating the Linux user $($S.LinuxUser)"
    Step "$status..."
    if (-not $S.UserExists) {
      Run $status (WSLExe) (AsRoot "useradd -m -s /bin/bash -G sudo $($S.LinuxUser)")
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
      $S.UserExists = $true
    }
    Run $status (WSLExe) (AsRoot ("f=/etc/wsl.conf; touch `$f`n" +
        "grep -q '^systemd=true' `$f || printf '\n[boot]\nsystemd=true\n' >> `$f`n" +
        "grep -q '^default=' `$f || printf '\n[user]\ndefault=$($S.LinuxUser)\n' >> `$f"))
    Run $status (WSLExe) "--terminate $Distro"
    MarkProvisioned
  }

  # InstallPackages installs the packages lerd's installer checks for, and turns
  # on linger, because with no terminal its offer to do both reads as "no" and it
  # stops without Podman. Keep the list in step with install.sh and the WSL guide.
  function InstallPackages {
    Step 'Installing Podman and the packages lerd needs...'
    Run 'Installing the packages' (WSLExe) (AsRoot ("export DEBIAN_FRONTEND=noninteractive`n" +
        'apt-get update -q && apt-get install -y -q podman uidmap fuse-overlayfs slirp4netns curl git unzip libnss3-tools && ' +
        "loginctl enable-linger $($S.LinuxUser)"))
  }

  # ---- desktop app -----------------------------------------------------------

  # On Windows the Lerd desktop app holds lerd's tray icon, raises its
  # notifications, which lerd-ui hands it while it runs, and keeps lerd's agent
  # running. It is downloaded from the latest lerd-desktop release and
  # registered with Windows here; the app has no installer of its own.

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

  # RegisterDesktopApp gives the app what Windows needs to treat it as lerd's:
  # the app id its notifications are posted under, with lerd's name and logo,
  # the lerd:// scheme that notification clicks open, a Start menu shortcut, and
  # a start at sign-in, hidden in the tray.
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

  # InstallDesktopApp downloads the app and unpacks it under Program Files. It
  # comes before lerd's wsl:setup, which leaves starting lerd's agent to the app
  # when it finds it registered, rather than to a Run entry of its own.
  # LERD_SETUP_LOCAL takes the app from a lerd-desktop-win32-<arch>.zip there
  # when it holds one; such a zip is unpacked on every run.
  function InstallDesktopApp {
    Step 'Installing the Lerd desktop app...'
    $arch = NativeArch
    $zipName = "lerd-desktop-win32-$arch.zip"
    $tag = ''
    $temp = @()
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
      $zip = Join-Path $env:TEMP 'lerd-desktop.zip'
      $temp += $zip
      Info "Downloading $($a.browser_download_url)"
      Invoke-WebRequest -UseBasicParsing $a.browser_download_url -OutFile $zip
    }
    # The logo Windows shows on lerd's notifications.
    $png = Join-Path $env:TEMP 'lerd.png'
    $temp += $png
    Invoke-WebRequest -UseBasicParsing "https://raw.githubusercontent.com/$Repo/main/internal/ui/icons/icon-512.png" -OutFile $png

    StopDesktopApp
    Info 'Unpacking the desktop app'
    # Windows' own tar unpacks zips. Its complaints are shown, since a
    # half-unpacked app opens to a black window. Errors are left to Continue
    # around it, or Windows PowerShell turns tar's first stderr line into a
    # terminating error and its exit code is never read.
    $tar = Join-Path $env:SystemRoot 'System32\tar.exe'
    $code = Elevated ("`$d = '$DesktopDir'; " +
      "if (Test-Path `$d) { Remove-Item `$d -Recurse -Force }; " +
      "Remove-Item '$DesktopVersionFile' -ErrorAction SilentlyContinue; " +
      "New-Item -ItemType Directory -Force `$d | Out-Null; " +
      "Copy-Item '$png' '$(Join-Path $AppDir 'lerd.png')' -Force; " +
      "`$ErrorActionPreference = 'Continue'; & '$tar' -xf '$zip' -C `$d 2>&1 | ForEach-Object { ""`$_"" }; " +
      "if (`$LASTEXITCODE -ne 0) { throw ""tar exited with `$LASTEXITCODE"" }; " +
      "if ('$tag') { Set-Content -Path '$DesktopVersionFile' -Value '$tag' -NoNewline }")
    $temp | ForEach-Object { Remove-Item $_ -ErrorAction SilentlyContinue }
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

  # ApplyOptions turns the answers into lerd commands. The installers took the
  # defaults (.test with HTTPS, lerd-managed Node), so only a change from them
  # runs anything. It comes before wsl:setup, which reads the DNS mode to decide
  # whether Windows needs the .test rule.
  function ApplyOptions {
    if ($S.LerdInstalled) { return }
    if ($S.DNSLocalhost) {
      Step 'Applying your options...'
      Run 'Applying your options' (WSLExe) (AsUser '~/.local/bin/lerd dns:disable')
    }
    if (-not $S.ManageNode) {
      Step 'Applying your options...'
      Run 'Applying your options' (WSLExe) (AsUser '~/.local/bin/lerd node:unmanage')
    }
  }

  # TrustCA imports lerd's root CA into the Windows user's Root store, which
  # makes Windows ask the user to confirm. wsl:setup does the same, but its
  # certutil starts deep in a chain of hidden processes and its dialog can open
  # behind this window. Once the CA is in the store, wsl:setup's import has
  # nothing left to ask. Declining is not a failure: the sites still work, with
  # a browser warning.
  function TrustCA {
    if (-not $S.LerdInstalled -and $S.DNSLocalhost) { return }
    $crt = Join-Path $env:TEMP 'lerd-rootCA.crt'
    # No CA means .localhost mode, which has no certificates to trust.
    $r = Capture (WSLExe) (AsUser "cp `"`$(~/.local/share/lerd/bin/mkcert -CAROOT)/rootCA.pem`" `"`$(wslpath '$crt')`"")
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
      $body = "cd `"`$(wslpath '$Local')`" && bash install.sh --local ./lerd"
    } else {
      $body = 'curl -fsSL https://lerd.sh/install.sh | bash'
    }
    Step "Installing lerd inside $Distro..."
    Run 'Installing lerd' (WSLExe) (AsUser $body)
    ApplyOptions
    TrustCA
    Step 'Connecting lerd with Windows...'
    Run 'Connecting lerd with Windows' (WSLExe) (AsUser '~/.local/bin/lerd wsl:setup')
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
  Remove-ItemProperty -Path $RunOnceKey -Name 'lerd-setup' -ErrorAction SilentlyContinue

  # The user is asked for after the restart, so it is not typed twice.
  if (-not (WSLFeaturesEnabled)) {
    try { EnableFeatures } catch { Warn $_.Exception.Message; Warn 'Run the installer again to retry.' }
    return
  }

  if ((WSLInstalled) -and (DistroInstalled)) { $S.LinuxUser = ExistingUser }
  $S.UserExists = [bool]$S.LinuxUser
  $S.LerdInstalled = $S.UserExists -and (LerdInstalledFor $S.LinuxUser)

  if ($S.UserExists) { Info "Using the existing Linux user $($S.LinuxUser)" } else { AskUser }
  if (-not $S.LerdInstalled) { AskOptions }

  try {
    EnsureWSL
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
