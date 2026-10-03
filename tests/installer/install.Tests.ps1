# Tests for install.ps1
# Requires: Pester 5+  (Install-Module Pester -Scope CurrentUser -Force -SkipPublisherCheck)
# Run:      Invoke-Pester tests/installer/install.Tests.ps1

BeforeAll {
  # Dot-sourcing loads the functions; the guard at the bottom keeps it from installing.
  . "$PSScriptRoot/../../install.ps1"

  # A stand-in lerd that records its arguments and exits with the given code.
  function New-FakeLerd([string]$dir, [int]$code) {
    $path = Join-Path $dir 'lerd.cmd'
    Set-Content -Path $path -Encoding ascii -Value "@echo %*> `"$dir\args.txt`"`r`n@exit /b $code"
    return $path
  }
}

Describe 'Get-LerdArch' {
  AfterEach {
    $env:PROCESSOR_ARCHITECTURE = $script:arch
    $env:PROCESSOR_ARCHITEW6432 = $script:arch6432
  }
  BeforeEach {
    $script:arch = $env:PROCESSOR_ARCHITECTURE
    $script:arch6432 = $env:PROCESSOR_ARCHITEW6432
    $env:PROCESSOR_ARCHITEW6432 = $null
  }

  It 'maps AMD64 to amd64' {
    $env:PROCESSOR_ARCHITECTURE = 'AMD64'
    Get-LerdArch | Should -Be 'amd64'
  }

  It 'maps ARM64 to arm64' {
    $env:PROCESSOR_ARCHITECTURE = 'ARM64'
    Get-LerdArch | Should -Be 'arm64'
  }

  It 'uses the real architecture from a 32-bit PowerShell' {
    $env:PROCESSOR_ARCHITECTURE = 'x86'
    $env:PROCESSOR_ARCHITEW6432 = 'AMD64'
    Get-LerdArch | Should -Be 'amd64'
  }

  It 'refuses a 32-bit Windows' {
    $env:PROCESSOR_ARCHITECTURE = 'x86'
    { Get-LerdArch } | Should -Throw '*Unsupported architecture: x86*'
  }
}

Describe 'Test-LerdVersion' {
  It 'accepts <v>' -ForEach @(@{ v = '1.30.0' }, @{ v = '1.34.0-beta.3' }) {
    Test-LerdVersion $v | Should -BeTrue
  }

  It 'refuses <v>' -ForEach @(@{ v = '1.0/../x' }, @{ v = '1.0;calc' }, @{ v = '' }) {
    Test-LerdVersion $v | Should -BeFalse
  }
}

Describe 'release version parsing' {
  It 'reads the tag from the releases/latest redirect' {
    ConvertFrom-ReleaseLocation 'https://github.com/lerd-env/lerd/releases/tag/v1.30.0' | Should -Be '1.30.0'
  }

  It 'returns nothing when the redirect is missing' {
    ConvertFrom-ReleaseLocation '' | Should -Be ''
  }

  It 'reads the newest tag from the atom feed' {
    $feed = '<link rel="alternate" href="https://github.com/lerd-env/lerd/releases/tag/v1.31.0-beta.2"/><link href="https://github.com/lerd-env/lerd/releases/tag/v1.30.0"/>'
    ConvertFrom-ReleaseFeed $feed | Should -Be '1.31.0-beta.2'
  }
}

Describe 'Get-ExpectedHash' {
  BeforeAll {
    $script:sums = "AAA111  lerd_1.30.0_linux_amd64.tar.gz`nBBB222  lerd_1.30.0_windows_amd64.zip`r`nccc333 *lerd_1.30.0_windows_arm64.zip`n"
  }

  It 'finds the line for the file, lowercased' {
    Get-ExpectedHash $sums 'lerd_1.30.0_windows_amd64.zip' | Should -Be 'bbb222'
  }

  It 'accepts the binary-mode marker' {
    Get-ExpectedHash $sums 'lerd_1.30.0_windows_arm64.zip' | Should -Be 'ccc333'
  }

  It 'returns nothing for a file it does not list' {
    Get-ExpectedHash $sums 'lerd_1.30.0_windows_386.zip' | Should -Be ''
  }
}

Describe 'Save-LerdRelease' {
  BeforeAll {
    $script:src = Join-Path $TestDrive 'src'
    New-Item -ItemType Directory -Path $src | Out-Null
    Set-Content -Path (Join-Path $src 'lerd.exe') -Value 'fake'
    $script:fixtureZip = Join-Path $TestDrive 'release.zip'
    Compress-Archive -Path (Join-Path $src '*') -DestinationPath $fixtureZip
    $script:hash = (Get-FileHash -Algorithm SHA256 $fixtureZip).Hash.ToLowerInvariant()
  }

  BeforeEach {
    $script:dest = Join-Path $TestDrive ([guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $dest | Out-Null
    Mock Invoke-WebRequest -ParameterFilter { $OutFile } { Copy-Item $fixtureZip $OutFile }
  }

  It 'extracts lerd.exe once the checksum matches' {
    Mock Invoke-WebRequest -ParameterFilter { -not $OutFile } { [pscustomobject]@{ Content = "$hash  lerd_1.30.0_windows_amd64.zip" } }
    $exe = Save-LerdRelease '1.30.0' 'amd64' $dest
    $exe | Should -Be (Join-Path $dest 'lerd.exe')
    $exe | Should -Exist
  }

  It 'downloads from the release of the requested version' {
    Mock Invoke-WebRequest -ParameterFilter { -not $OutFile } { [pscustomobject]@{ Content = "$hash  lerd_1.30.0_windows_amd64.zip" } }
    Save-LerdRelease '1.30.0' 'amd64' $dest | Out-Null
    Should -Invoke Invoke-WebRequest -ParameterFilter { $Uri -eq 'https://github.com/lerd-env/lerd/releases/download/v1.30.0/lerd_1.30.0_windows_amd64.zip' }
  }

  It 'refuses an archive whose checksum does not match' {
    Mock Invoke-WebRequest -ParameterFilter { -not $OutFile } { [pscustomobject]@{ Content = "deadbeef  lerd_1.30.0_windows_amd64.zip" } }
    { Save-LerdRelease '1.30.0' 'amd64' $dest } | Should -Throw '*Checksum mismatch*'
    Join-Path $dest 'lerd.exe' | Should -Not -Exist
  }

  It 'refuses a release whose checksums do not list the archive' {
    Mock Invoke-WebRequest -ParameterFilter { -not $OutFile } { [pscustomobject]@{ Content = "$hash  lerd_1.30.0_linux_amd64.tar.gz" } }
    { Save-LerdRelease '1.30.0' 'amd64' $dest } | Should -Throw '*no entry for lerd_1.30.0_windows_amd64.zip*'
  }
}

Describe 'Install-Lerd' {
  BeforeEach {
    $script:work = Join-Path $TestDrive ([guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $work | Out-Null
    $Version = ''
    $Beta = $false
    $Local = ''
  }

  It 'runs lerd install with the local binary' {
    $Local = New-FakeLerd $work 0
    Install-Lerd *> $null
    (Get-Content (Join-Path $work 'args.txt')).Trim() | Should -Be 'install'
  }

  It 'tells the user how to resume when lerd install stops' {
    $Local = New-FakeLerd $work 1
    { Install-Lerd *> $null } | Should -Throw '*lerd\bin\lerd.exe" install*'
  }

  It 'skips the download when that version is already installed' {
    Mock Get-LerdArch { 'amd64' }
    Mock Resolve-LerdVersion { '1.30.0' }
    Mock Get-InstalledLerdVersion { '1.30.0' }
    Mock Save-LerdRelease {}
    Install-Lerd *> $null
    Should -Invoke Save-LerdRelease -Times 0
  }

  It 'refuses an unsafe version before downloading' {
    Mock Get-LerdArch { 'amd64' }
    Mock Resolve-LerdVersion { '1.0/../evil' }
    Mock Save-LerdRelease {}
    { Install-Lerd *> $null } | Should -Throw "*unsafe release version*"
    Should -Invoke Save-LerdRelease -Times 0
  }

  It 'removes its temp folder afterwards' {
    $Local = New-FakeLerd $work 0
    $before = @(Get-ChildItem ([IO.Path]::GetTempPath()) -Filter 'lerd-install-*' -Directory).Count
    Install-Lerd *> $null
    @(Get-ChildItem ([IO.Path]::GetTempPath()) -Filter 'lerd-install-*' -Directory).Count | Should -Be $before
  }
}

Describe 'Resolve-LerdVersion' {
  It 'uses -Version as given, without a leading v' {
    $Version = 'v1.29.1'
    Resolve-LerdVersion | Should -Be '1.29.1'
  }

  It 'follows the prerelease line under -Beta' {
    $Version = ''
    $Beta = $true
    Mock Get-LatestPrerelease { '1.31.0-beta.1' }
    Resolve-LerdVersion | Should -Be '1.31.0-beta.1'
  }

  It 'takes the newest stable release by default' {
    $Version = ''
    $Beta = $false
    Mock Get-LatestVersion { '1.30.0' }
    Resolve-LerdVersion | Should -Be '1.30.0'
  }
}
