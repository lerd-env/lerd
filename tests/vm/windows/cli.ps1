# Drives lerd.exe and its shim twins from Windows the way a user would, from
# PowerShell and from cmd, in a distro folder and on a drive, one PASS or FAIL
# line each. Needs a lane run before it: a linked Laravel site in -Site.
param(
	[string] $Distro = (Get-Content "$env:LOCALAPPDATA\lerd\bin\lerd-wsl.conf" | Where-Object { $_ -like 'distro=*' } | ForEach-Object { $_.Substring(7) }),
	[string] $User = $env:USERNAME,
	[string] $Site = 'demo'
)
$ErrorActionPreference = 'Continue'
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$script:fails = 0
$script:passes = 0

function Check([string] $name, [bool] $ok, [string] $got = '') {
	if ($ok) { $script:passes++; "PASS $name" } else { $script:fails++; "FAIL $name$(if ($got) { " (got: $($got.Trim() -replace '\s+', ' ' | ForEach-Object { $_.Substring(0, [Math]::Min(200, $_.Length)) }))" })" }
}

# In runs a native command from dir and returns its stdout, or with -All its
# stderr too, which lerd uses for notes such as a service it starts on the way;
# $LASTEXITCODE stays set.
function In([string] $dir, [scriptblock] $cmd, [switch] $All) {
	Push-Location -LiteralPath $dir
	try {
		if ($All) { (& $cmd 2>&1 | ForEach-Object { "$_" } | Out-String) } else { (& $cmd 2>$null | Out-String) }
	} finally { Pop-Location }
}

# Cmd runs a cmd.exe line from dir, entered with pushd as a cmd user must for a
# \\wsl.localhost folder; the line's exit code comes back as the last line.
function Cmd([string] $dir, [string] $line) {
	(cmd.exe /d /c "pushd `"$dir`" && $line & call echo EXIT=%^ERRORLEVEL%" 2>&1 | Out-String)
}

$wsl = 'C:\Program Files\WSL\wsl.exe'
$bin = Join-Path $env:LOCALAPPDATA 'lerd\bin'
$linuxHome = "/home/$User"
$unc = "\\wsl.localhost\$Distro\home\$User\Projects\$Site"
$uncDollar = "\\wsl$\$Distro\home\$User\Projects\$Site"
$win = Join-Path $env:USERPROFILE 'lerd-cli-test'
New-Item -ItemType Directory -Force $win | Out-Null
$env:Path = "$bin;$env:Path"

function InDistro([string] $line) { (& $wsl -d $Distro -u $User --cd $linuxHome --exec bash -lc $line 2>$null | Out-String) }

# --- lerd.exe itself
$want = (InDistro '~/.local/bin/lerd --version').Trim()
$got = In $win { lerd --version }
Check 'C.1 lerd --version is the distro''s lerd' ($got.Trim() -eq $want) $got
$got = In $win { lerd }
Check 'C.2 bare lerd prints the command list' ($got -match 'Available Commands') $got
$got = In $win { lerd no-such-command } -All
Check 'C.3 an unknown command fails with its exit code' ($LASTEXITCODE -ne 0 -and $got -match 'unknown command') "$LASTEXITCODE $got"

# --- every command's help through lerd.exe matches the distro's, exit code too:
# the ones that wrap a tool hand --help to it, which can fail outside a site
$cmds = (InDistro '~/.local/bin/lerd --help') -split "`n" |
	Where-Object { $_ -match '^\s{2}(\S+)\s{2,}' } | ForEach-Object { $Matches[1] }
foreach ($c in $cmds) {
	$want = InDistro "~/.local/bin/lerd $c --help"
	$wantExit = $LASTEXITCODE
	$got = In $win { lerd $c --help }
	Check "C.4 lerd $c --help" ($LASTEXITCODE -eq $wantExit -and $got.Trim() -eq $want.Trim()) "exit $LASTEXITCODE, distro $wantExit"
}

# --- site context from a distro folder
foreach ($dir in @($unc, $uncDollar)) {
	$got = In $dir { lerd which }
	Check "C.5 lerd which names the site from $dir" ($got -match "Projects/$Site") $got
}
$got = In $unc { lerd php -v }
Check 'C.6 lerd php -v' ($got -match '^PHP \d') $got
$got = In $unc { php -v }
Check 'C.7 php.exe -v' ($got -match '^PHP \d') $got
$got = In $unc { php artisan --version }
Check 'C.8 php artisan from the site folder' ($got -match 'Laravel Framework') $got
$got = In $unc { php .\artisan --version }
Check 'C.8b php .\artisan, a relative Windows path' ($got -match 'Laravel Framework') $got
$got = In $unc { composer --version }
Check 'C.9 composer.exe --version' ($got -match 'Composer version') $got
$got = In $unc { node -v }
Check 'C.10 node.exe -v' ($got -match '^v\d+') $got
$got = In $unc { npm -v }
Check 'C.10 npm.exe -v' ($got -match '^\d+\.\d+') $got
$got = In $unc { npx --version }
Check 'C.10 npx.exe --version' ($got -match '^\d+\.\d+') $got
$got = In $unc { laravel --version }
Check 'C.11 laravel.exe --version' ($got -match 'Laravel Installer') $got
$got = In $unc { '<?php echo 40+2;' | php }
Check 'C.12 stdin piped into php' ($got.Trim() -eq '42') $got
$got = In $unc { php -r "echo 'a b';" }
Check 'C.13 an argument with spaces arrives whole' ($got.Trim() -eq 'a b') $got
$null = In $unc { php -r 'exit(7);' }
Check 'C.14 the exit code comes back' ($LASTEXITCODE -eq 7) "$LASTEXITCODE"

# --- Windows paths as arguments
Set-Content -Encoding ascii (Join-Path $win 't.php') '<?php echo "from windows";'
$got = In $unc { php "$win\t.php" }
Check 'C.15 a C:\ path argument reaches php' ($got.Trim() -eq 'from windows') $got
$got = In $win { composer validate --no-check-publish --no-check-all "--working-dir=$unc" }
Check 'C.16 a \\wsl.localhost path in --flag=' ($LASTEXITCODE -eq 0 -and $got -match 'valid') $got
$dump = Join-Path $win 'cli.sql'
Remove-Item -Force -ErrorAction SilentlyContinue $dump
$got = In $unc { lerd db:export -o $dump } -All
Check 'C.17 db:export writes to a Windows path' ((Test-Path $dump) -and (Get-Item $dump).Length -gt 0) $got

# --- lerd's own commands from PowerShell
foreach ($c in 'status', 'sites', 'doctor', 'php:list', 'service list', 'env:check', 'site:doctor') {
	$got = In $unc { lerd @($c -split ' ') } -All
	Check "C.18 lerd $c" ($LASTEXITCODE -eq 0) "exit $LASTEXITCODE $got"
}

# --- link and unlink a new site from a distro folder, reached from Windows;
# empty stdin, since with a console link asks which framework this is
$null = InDistro 'mkdir -p ~/Projects/clitest && echo ''<?php echo 40+2, chr(32), 1+1;'' > ~/Projects/clitest/index.php'
$new = "\\wsl.localhost\$Distro\home\$User\Projects\clitest"
$got = In $new { '' | lerd link } -All
Check 'C.19 lerd link from a distro folder' ($LASTEXITCODE -eq 0) $got
# link serves plain HTTP; secure issues the cert Windows trusts through the CA.
$got = In $new { '' | lerd secure } -All
Check 'C.19 lerd secure from the same folder' ($LASTEXITCODE -eq 0) $got
$body = try { (Invoke-WebRequest -UseBasicParsing -TimeoutSec 60 'https://clitest.test/').Content } catch { "$_" }
Check 'C.19 the new site answers in a Windows browser request' ($body -match '42 2') $body
$got = In $new { '' | lerd unlink } -All
Check 'C.19 lerd unlink from the same folder' ($LASTEXITCODE -eq 0 -and (InDistro '~/.local/bin/lerd sites') -notmatch 'clitest') $got
$null = InDistro 'rm -rf ~/Projects/clitest'

# --- service client shims get a Windows twin
# Windows PowerShell drops the double quotes inside a native argument, so no
# InDistro line has any, and its output ends lines with CRLF. The agent mirrors a new shim within half a minute.
$shims = (InDistro 'cd ~/.local/share/lerd/bin && for f in *; do case $f in *.*) ;; *) head -c2 $f | grep -q ^#! && echo $f ;; esac; done') -split '\r?\n' | Where-Object { $_ -match '^[\w-]+$' }
Check 'C.20 lerd has shims to mirror' ($shims.Count -gt 0)
foreach ($s in $shims) {
	Check "C.20 $s.exe on the Windows PATH" (Test-Path (Join-Path $bin "$s.exe"))
}
$got = In $unc { psql --version }
Check 'C.20 psql.exe, a service''s client tool, runs' ($got -match 'psql') $got

# --- output read through a pipe, as a script would, without forcing UTF-8
$got = (powershell -NoProfile -Command "Set-Location -LiteralPath '$unc'; lerd status" | Out-String)
Check 'C.21 lerd status piped in a fresh Windows PowerShell keeps its symbols' ($got -match '✓') ($got -split "`n" | Where-Object { $_ -match 'resolution' } | Select-Object -First 1)

# --- cmd.exe
$got = Cmd $win 'lerd --version'
Check 'K.1 cmd: lerd --version' ($got -match 'lerd version' -and $got -match 'EXIT=0') $got
$got = Cmd $unc 'lerd which'
Check 'K.2 cmd: lerd which after pushd to the site' ($got -match "Projects/$Site" -and $got -match 'EXIT=0') $got
$got = Cmd $unc 'php artisan --version'
Check 'K.3 cmd: php artisan after pushd' ($got -match 'Laravel Framework') $got
$got = Cmd $unc 'php .\artisan --version'
Check 'K.3b cmd: php .\artisan after pushd' ($got -match 'Laravel Framework') $got
$got = Cmd $unc 'composer --version'
Check 'K.4 cmd: composer --version' ($got -match 'Composer version') $got
$got = Cmd $win 'php -r "exit(7);"'
Check 'K.5 cmd: the exit code comes back' ($got -match 'EXIT=7') $got
$got = Cmd $win "php `"$win\t.php`""
Check 'K.6 cmd: a C:\ path argument reaches php' ($got -match 'from windows') $got
$got = Cmd $win "type `"$win\t.php`" | php"
Check 'K.7 cmd: stdin piped into php' ($got -match 'from windows') $got

Remove-Item -Recurse -Force $win
"SUMMARY windows cli: $script:passes passed, $script:fails failed"
