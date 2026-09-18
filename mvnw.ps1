#Requires -Version 5.0
$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Props = Join-Path $Root '.mvn\wrapper\maven-wrapper.properties'
$distUrl = 'https://repo.maven.apache.org/maven2/org/apache/maven/apache-maven/3.9.9/apache-maven-3.9.9-bin.zip'
if (Test-Path $Props) {
  foreach ($line in Get-Content $Props) {
    if ($line -match '^distributionUrl=(.+)$') {
      $distUrl = $Matches[1].Trim()
    }
  }
}
$name = [IO.Path]::GetFileNameWithoutExtension(([Uri]$distUrl).Segments[-1]) -replace '-bin$', ''
$cache = Join-Path $env:USERPROFILE ".m2\wrapper\dists\$name"
$mvn = Join-Path $cache "bin\mvn.cmd"
if (-not (Test-Path $mvn)) {
  New-Item -ItemType Directory -Force -Path $cache | Out-Null
  $zip = Join-Path $env:TEMP "$name-bin.zip"
  Write-Host "Downloading $distUrl"
  Invoke-WebRequest -Uri $distUrl -OutFile $zip
  $extract = Join-Path $env:TEMP "$name-extract"
  if (Test-Path $extract) { Remove-Item -Recurse -Force $extract }
  Expand-Archive -Path $zip -DestinationPath $extract -Force
  $inner = Get-ChildItem $extract -Directory | Select-Object -First 1
  Copy-Item -Path (Join-Path $inner.FullName '*') -Destination $cache -Recurse -Force
}
& $mvn -f (Join-Path $Root 'pom.xml') @args
exit $LASTEXITCODE
