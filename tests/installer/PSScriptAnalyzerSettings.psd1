# PSScriptAnalyzer settings for install.ps1. Write-Host is the installer's
# output, and the script-level parameters are read inside its functions, which
# the unused-parameter rule cannot follow.
@{
  Severity     = @('ParseError', 'Error', 'Warning')
  ExcludeRules = @('PSAvoidUsingWriteHost', 'PSReviewUnusedParameter')
  Rules        = @{
    # Windows PowerShell 5.1 is what every Windows ships, so nothing newer may slip in.
    PSUseCompatibleSyntax = @{
      Enable         = $true
      TargetVersions = @('5.1', '7.0')
    }
  }
}
