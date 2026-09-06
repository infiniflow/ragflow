param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+$')]
    [string]$ReleaseVersion,
    [string]$OutputDirectory = $PSScriptRoot,
    [switch]$UseExistingFrontend,
    [string]$CandidateDirectory,
    [string]$FrontendArtifact,
    [string]$BundleCacheDirectory,
    [string]$PythonCommand,
    [switch]$Overwrite
)

$ErrorActionPreference = 'Stop'
$sourceRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$toolRoot = $sourceRoot
$verifiedMode = [bool]($CandidateDirectory -or $FrontendArtifact -or $BundleCacheDirectory)
if ($verifiedMode -and (-not $CandidateDirectory -or -not $FrontendArtifact -or -not $BundleCacheDirectory)) {
    throw 'Verified offline mode requires CandidateDirectory, FrontendArtifact and BundleCacheDirectory together.'
}
function Invoke-QualityTool {
    param([string]$ToolName, [string[]]$ArgumentList)
    $result = & $PythonCommand (Join-Path $toolRoot "tools/quality/$ToolName") @ArgumentList
    if ($LASTEXITCODE -ne 0) {
        throw "Quality tool $ToolName rejected the inputs (exit $LASTEXITCODE)."
    }
    return ($result -join "`n")
}
if ($verifiedMode) {
    if (-not $PythonCommand) {
        $localPython = Join-Path $toolRoot '.venv/Scripts/python.exe'
        $unixPython = Join-Path $toolRoot '.venv/bin/python'
        $PythonCommand = if (Test-Path -LiteralPath $localPython) { $localPython }
            elseif (Test-Path -LiteralPath $unixPython) { $unixPython }
            else { (Get-Command python3 -ErrorAction Stop).Source }
    }
    $CandidateDirectory = [System.IO.Path]::GetFullPath($CandidateDirectory)
    $FrontendArtifact = [System.IO.Path]::GetFullPath($FrontendArtifact)
    $BundleCacheDirectory = [System.IO.Path]::GetFullPath($BundleCacheDirectory)
    if ($BundleCacheDirectory -eq $CandidateDirectory -or
        $BundleCacheDirectory.StartsWith($CandidateDirectory + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase) -or
        $CandidateDirectory.StartsWith($BundleCacheDirectory + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw 'BundleCacheDirectory and CandidateDirectory must be separate paths.'
    }
    $null = Invoke-QualityTool 'candidate.py' @('verify', '--candidate', $CandidateDirectory)
    $candidateIdentity = Get-Content -LiteralPath (Join-Path $CandidateDirectory 'candidate.json') -Raw | ConvertFrom-Json
    if ($ReleaseVersion -ne "v$($candidateIdentity.identity.version)") { throw 'ReleaseVersion does not match candidate identity.' }
    $null = Invoke-QualityTool 'frontend_artifact.py' @('verify', '--candidate', $CandidateDirectory, '--archive', $FrontendArtifact)
    $sourceRoot = Join-Path $CandidateDirectory 'source'
}
else {
    Write-Warning 'Legacy offline packaging uses unverified workspace/frontend inputs; no candidate or artifact reuse proof is asserted.'
}
$deliveryRoot = Join-Path $sourceRoot 'deployment/linux-pg'
$outputRoot = [System.IO.Path]::GetFullPath($OutputDirectory)
if ($verifiedMode -and ($outputRoot -eq $sourceRoot -or
    $outputRoot.StartsWith($sourceRoot + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase))) {
    throw 'OutputDirectory must not modify candidate source.'
}
$archiveName = "ragflow-linux-pg-$ReleaseVersion-offline.tar.gz"
$archivePath = Join-Path $outputRoot $archiveName
$checksumPath = $archivePath + '.sha256'
$sourceArchiveName = "ragflow-linux-pg-$ReleaseVersion.tar.gz"

if (-not $Overwrite -and ((Test-Path -LiteralPath $archivePath) -or (Test-Path -LiteralPath $checksumPath))) {
    throw "Refusing to overwrite an existing offline release: $archivePath"
}

$requiredCommands = @('docker', 'tar')
if (-not $UseExistingFrontend -and -not $verifiedMode) { $requiredCommands += 'pnpm.cmd' }
foreach ($commandName in $requiredCommands) {
    if (-not (Get-Command $commandName -ErrorAction SilentlyContinue)) {
        throw "Required build command is missing: $commandName"
    }
}
$tarCommand = Get-Command tar -ErrorAction Stop
function Invoke-Tar {
    param(
        [Parameter(Mandatory = $true)][string[]]$ArgumentList,
        [Parameter(Mandatory = $true)][string]$ErrorMessage,
        [switch]$DiscardOutput
    )

    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $tarCommand.Source
    $startInfo.UseShellExecute = $false
    $startInfo.RedirectStandardOutput = $DiscardOutput.IsPresent
    foreach ($argument in $ArgumentList) {
        [void]$startInfo.ArgumentList.Add($argument)
    }
    $process = [System.Diagnostics.Process]::Start($startInfo)
    if ($DiscardOutput) {
        $null = $process.StandardOutput.ReadToEnd()
    }
    $process.WaitForExit()
    if ($process.ExitCode -ne 0) {
        throw $ErrorMessage
    }
}

function Write-LfText {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string[]]$Lines
    )

    $content = ($Lines -join "`n") + "`n"
    [System.IO.File]::WriteAllText($Path, $content, [System.Text.UTF8Encoding]::new($false))
}

function Copy-LfText {
    param(
        [Parameter(Mandatory = $true)][string]$Source,
        [Parameter(Mandatory = $true)][string]$Destination
    )

    $content = [System.IO.File]::ReadAllText($Source).
        Replace("`r`n", "`n").
        Replace("`r", "`n")
    [System.IO.File]::WriteAllText($Destination, $content, [System.Text.UTF8Encoding]::new($false))
}

$projectVersionMatch = [regex]::Match(
    (Get-Content -LiteralPath (Join-Path $sourceRoot 'pyproject.toml') -Raw),
    '(?m)^version\s*=\s*"([^"]+)"'
)
if (-not $projectVersionMatch.Success -or $projectVersionMatch.Groups[1].Value -ne $ReleaseVersion.TrimStart('v')) {
    throw "ReleaseVersion $ReleaseVersion does not match pyproject.toml version."
}

$asrImage = "ragflow/t-one-asr:$($ReleaseVersion.TrimStart('v'))"
$dockerImages = @(
    'postgres:16-alpine',
    'infiniflow/ragflow:v0.26.4',
    'valkey/valkey:8',
    'elasticsearch:8.11.3',
    'plantuml/plantuml-server:jetty-v1.2026.6',
    'pgsty/minio:RELEASE.2026-03-25T00-00-00Z',
    $asrImage,
    'otel/opentelemetry-collector-contrib:0.160.0',
    'grafana/tempo:2.10.5',
    'grafana/loki:3.7.0',
    'prom/prometheus:v3.11.0',
    'grafana/grafana:13.1.0',
    'infiniflow/sandbox-executor-manager:latest',
    'infiniflow/sandbox-base-nodejs:latest',
    'infiniflow/sandbox-base-python:latest'
)

$tempBase = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
$tempRoot = Join-Path $tempBase ('ragflow-linux-pg-offline-' + [guid]::NewGuid().ToString('N'))
$packageRoot = Join-Path $tempRoot 'package'
$payloadRoot = Join-Path $packageRoot 'payload'
$sourceArchivePath = Join-Path $payloadRoot $sourceArchiveName
$sourceChecksumPath = $sourceArchivePath + '.sha256'
$dockerArchivePath = Join-Path $payloadRoot 'docker-images.tar'
$frontendArchivePath = Join-Path $payloadRoot 'web-dist.tar.gz'
$gvisorBundlePath = Join-Path $payloadRoot 'gvisor'

New-Item -ItemType Directory -Path $payloadRoot -Force | Out-Null
New-Item -ItemType Directory -Path $outputRoot -Force | Out-Null

try {
    $sourceArguments = @{ ReleaseVersion = $ReleaseVersion; OutputDirectory = $payloadRoot; Overwrite = $true }
    if ($verifiedMode) {
        $sourceArguments.CandidateDirectory = $CandidateDirectory
        $sourceArguments.PythonCommand = $PythonCommand
        $sourceCache = Join-Path $BundleCacheDirectory "source-$($candidateIdentity.source_id)"
        $cachedSourceArchive = Join-Path $sourceCache $sourceArchiveName
        $cachedSourceChecksum = $cachedSourceArchive + '.sha256'
        $sourceArguments.OutputDirectory = $sourceCache
        $sourceArguments.Overwrite = $false
        if (Test-Path -LiteralPath $cachedSourceArchive) {
            Write-Host "Reusing verified source cache: $sourceCache"
        }
        else {
            if ((Test-Path -LiteralPath (Join-Path $CandidateDirectory 'artifact-linux-pg-source.json')) -or
                (Test-Path -LiteralPath $cachedSourceChecksum)) {
                throw 'Canonical source archive is missing from cache; recover its recorded bytes or use a new candidate.'
            }
            & (Join-Path $PSScriptRoot 'build_archive.ps1') @sourceArguments
        }
        $null = Invoke-QualityTool 'candidate.py' @('verify-artifact', '--candidate', $CandidateDirectory, '--artifact', $cachedSourceArchive, '--name', 'linux-pg-source')
        $sourceHash = (Get-FileHash -LiteralPath $cachedSourceArchive -Algorithm SHA256).Hash.ToLowerInvariant()
        $sourceChecksumText = (Get-Content -LiteralPath $cachedSourceChecksum -Raw).TrimEnd("`r", "`n")
        if ($sourceChecksumText -ne "$sourceHash  $sourceArchiveName") { throw 'Cached source checksum sidecar mismatch.' }
        $sourceMetadata = (& $tarCommand.Source -xOf $cachedSourceArchive './DEPLOYMENT-SOURCE.env') -join "`n"
        if ($LASTEXITCODE -ne 0 -or $sourceMetadata -notmatch '(?m)^FRONTEND_MODE=excluded$' -or
            $sourceMetadata -notmatch "(?m)^SOURCE_ID=$($candidateIdentity.source_id)$") {
            throw 'Cached canonical source must be source-only and match candidate SOURCE_ID.'
        }
        Copy-Item -LiteralPath $cachedSourceArchive -Destination $sourceArchivePath
        Copy-Item -LiteralPath $cachedSourceChecksum -Destination $sourceChecksumPath
        $null = Invoke-QualityTool 'candidate.py' @('verify-artifact', '--candidate', $CandidateDirectory, '--artifact', $sourceArchivePath, '--name', 'linux-pg-source')
    }
    else {
        & (Join-Path $PSScriptRoot 'build_archive.ps1') @sourceArguments
    }

    if ($verifiedMode) {
        $verifiedFrontend = Join-Path $tempRoot 'frontend.tar.gz'
        Copy-Item -LiteralPath $FrontendArtifact -Destination $verifiedFrontend
        Copy-Item -LiteralPath ($FrontendArtifact + '.json') -Destination ($verifiedFrontend + '.json')
        $null = Invoke-QualityTool 'frontend_artifact.py' @('verify', '--candidate', $CandidateDirectory, '--archive', $verifiedFrontend)
        $frontendRoot = Join-Path $tempRoot 'frontend'
        New-Item -ItemType Directory -Path (Join-Path $frontendRoot 'dist') -Force | Out-Null
        Invoke-Tar -ArgumentList @('-xzf', $verifiedFrontend, '-C', (Join-Path $frontendRoot 'dist')) `
            -ErrorMessage 'Verified frontend extraction failed.'
        Copy-Item -LiteralPath ($verifiedFrontend + '.json') -Destination (Join-Path $payloadRoot 'frontend-build.json')
    }
    elseif (-not $UseExistingFrontend) {
        Push-Location (Join-Path $sourceRoot 'web')
        try {
            & pnpm.cmd install --frozen-lockfile --ignore-scripts
            if ($LASTEXITCODE -ne 0) {
                throw 'Frontend dependency installation failed.'
            }
            & pnpm.cmd run build
            if ($LASTEXITCODE -ne 0) {
                throw 'Frontend production build failed.'
            }
        }
        finally {
            Pop-Location
        }
    }
    if (-not $verifiedMode) { $frontendRoot = Join-Path $sourceRoot 'web' }
    if (-not (Test-Path -LiteralPath (Join-Path $frontendRoot 'dist/index.html') -PathType Leaf)) {
        throw 'Frontend build did not create web/dist/index.html.'
    }

    & docker build --platform linux/amd64 --tag $asrImage (Join-Path $sourceRoot 'services\asr-online-service')
    if ($LASTEXITCODE -ne 0) {
        throw 'T-One ASR image build failed.'
    }
    & (Join-Path $deliveryRoot 'prepare_gvisor_bundle.ps1') -Destination $gvisorBundlePath

    foreach ($imageName in $dockerImages) {
        $platform = (& docker image inspect $imageName --format '{{.Os}}/{{.Architecture}}').Trim()
        if ($LASTEXITCODE -ne 0) {
            throw "Required Docker image is missing: $imageName"
        }
        if ($platform -ne 'linux/amd64') {
            throw "Docker image has unsupported platform ${platform}: $imageName"
        }
    }

    Copy-LfText -Source (Join-Path $deliveryRoot 'install_offline.sh') -Destination (Join-Path $packageRoot 'install_offline.sh')
    Copy-LfText -Source (Join-Path $deliveryRoot 'upgrade_offline.sh') -Destination (Join-Path $packageRoot 'upgrade_offline.sh')

    Invoke-Tar -ArgumentList @('-czf', $frontendArchivePath, '-C', $frontendRoot, 'dist') `
        -ErrorMessage 'Frontend archive creation failed.'

    if ($verifiedMode) {
        $imageArguments = @()
        foreach ($imageName in $dockerImages) { $imageArguments += @('--image', $imageName) }
        $bundle = Invoke-QualityTool 'docker_bundle.py' (@('materialize', '--candidate', $CandidateDirectory, '--cache', $BundleCacheDirectory) + $imageArguments) | ConvertFrom-Json
        Copy-Item -LiteralPath $bundle.archive -Destination $dockerArchivePath
        $dockerReceiptPath = Join-Path $payloadRoot 'docker-images.json'
        Copy-Item -LiteralPath $bundle.receipt -Destination $dockerReceiptPath
        $null = Invoke-QualityTool 'docker_bundle.py' (@('verify', '--candidate', $CandidateDirectory, '--archive', $dockerArchivePath, '--receipt', $dockerReceiptPath) + $imageArguments)
    }
    else {
        & docker image save --output $dockerArchivePath @dockerImages
        if ($LASTEXITCODE -ne 0) { throw 'Docker image archive creation failed.' }
    }
    Write-LfText -Path (Join-Path $payloadRoot 'docker-images.txt') -Lines $dockerImages

    $manifestLines = @(
        "RELEASE_VERSION=$ReleaseVersion"
        'PACKAGE_MODE=offline'
        'PACKAGE_FORMAT=tar.gz'
        'TARGET_OS=rocky'
        'TARGET_VERSION=9'
        'TARGET_ARCH=amd64'
        'DOCKER_DNF_REPO=cifra-docker'
        "SOURCE_ARCHIVE=$sourceArchiveName"
        'FRONTEND_ARCHIVE=web-dist.tar.gz'
        'DOCKER_IMAGES_ARCHIVE=docker-images.tar'
        'GVISOR_BUNDLE=gvisor'
        "DOCKER_IMAGE_COUNT=$($dockerImages.Count)"
        "PACKAGED_AT_UTC=$([DateTime]::UtcNow.ToString('o'))"
    )
    if ($verifiedMode) {
        $manifestLines += "SOURCE_ID=$($candidateIdentity.source_id)"
        $manifestLines += 'CANDIDATE_VALIDATION=NOT_ASSERTED'
        $manifestLines += 'FRONTEND_RECEIPT=frontend-build.json'
        $manifestLines += 'DOCKER_IMAGES_RECEIPT=docker-images.json'
    }
    Write-LfText -Path (Join-Path $packageRoot 'OFFLINE-PACKAGE.env') -Lines $manifestLines

    $checksumLines = @(
        Get-ChildItem -LiteralPath $payloadRoot -Recurse -File | Sort-Object FullName | ForEach-Object {
            $relativePath = [System.IO.Path]::GetRelativePath($packageRoot, $_.FullName).Replace('\', '/')
            $fileHash = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
            "$fileHash  $relativePath"
        }
    )
    $checksumsPath = Join-Path $packageRoot 'SHA256SUMS'
    Write-LfText -Path $checksumsPath -Lines $checksumLines

    foreach ($metadataPath in @(
        (Join-Path $packageRoot 'OFFLINE-PACKAGE.env'),
        (Join-Path $payloadRoot 'docker-images.txt'),
        $sourceChecksumPath,
        $checksumsPath
    )) {
        if ([System.IO.File]::ReadAllText($metadataPath).Contains("`r")) {
            throw "Linux metadata contains a CR character: $metadataPath"
        }
    }
    foreach ($checksumLine in $checksumLines) {
        if ($checksumLine -notmatch '^([0-9a-f]{64})  (.+)$') {
            throw "Invalid checksum line: $checksumLine"
        }
        $payloadPath = Join-Path $packageRoot $Matches[2].Replace('/', [System.IO.Path]::DirectorySeparatorChar)
        $actualPayloadHash = (Get-FileHash -LiteralPath $payloadPath -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actualPayloadHash -ne $Matches[1]) {
            throw "Payload checksum verification failed: $($Matches[2])"
        }
    }

    $archiveEntries = @(
        'OFFLINE-PACKAGE.env'
        'SHA256SUMS'
        'install_offline.sh'
        'upgrade_offline.sh'
    ) + @(
        Get-ChildItem -LiteralPath $payloadRoot -Recurse -File | Sort-Object FullName | ForEach-Object {
            'payload/' + [System.IO.Path]::GetRelativePath($payloadRoot, $_.FullName).Replace('\', '/')
        }
    )
    if ($verifiedMode) {
        $null = Invoke-QualityTool 'candidate.py' @('verify', '--candidate', $CandidateDirectory)
        $null = Invoke-QualityTool 'candidate.py' @('verify-artifact', '--candidate', $CandidateDirectory, '--artifact', $sourceArchivePath, '--name', 'linux-pg-source')
        $currentIdentity = Get-Content -LiteralPath (Join-Path $CandidateDirectory 'candidate.json') -Raw | ConvertFrom-Json
        if ($currentIdentity.source_id -ne $candidateIdentity.source_id) { throw 'Candidate changed during offline packaging.' }
        $null = Invoke-QualityTool 'frontend_artifact.py' @('verify', '--candidate', $CandidateDirectory, '--archive', $verifiedFrontend)
        $null = Invoke-QualityTool 'frontend_artifact.py' @('verify', '--candidate', $CandidateDirectory, '--archive', $FrontendArtifact)
        $null = Invoke-QualityTool 'docker_bundle.py' (@('verify', '--candidate', $CandidateDirectory, '--archive', $dockerArchivePath, '--receipt', $dockerReceiptPath) + $imageArguments)
    }
    Invoke-Tar -ArgumentList (@('-czf', $archivePath, '-C', $packageRoot) + $archiveEntries) `
        -ErrorMessage 'Offline archive creation failed.'
    Invoke-Tar -ArgumentList @('-tzf', $archivePath) `
        -ErrorMessage 'Offline archive integrity check failed.' `
        -DiscardOutput

    $archiveHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    Write-LfText -Path $checksumPath -Lines @("$archiveHash  $archiveName")

    Write-Host "Offline archive: $archivePath"
    Write-Host "Checksum: $checksumPath"
    Write-Host "SHA256: $archiveHash"
    Write-Host "Size: $((Get-Item -LiteralPath $archivePath).Length) bytes"
    Write-Host "Docker images: $($dockerImages.Count)"
    Write-Host 'Target: Rocky Linux 9.x with Docker packages from cifra-docker'
}
finally {
    $resolvedTemp = [System.IO.Path]::GetFullPath($tempRoot)
    if ($resolvedTemp.StartsWith($tempBase, [System.StringComparison]::OrdinalIgnoreCase) -and
        (Split-Path -Leaf $resolvedTemp) -like 'ragflow-linux-pg-offline-*') {
        Remove-Item -LiteralPath $resolvedTemp -Recurse -Force -ErrorAction SilentlyContinue
    }
}
