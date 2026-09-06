param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[0-9A-Za-z][0-9A-Za-z._-]*$')]
    [string]$ReleaseVersion,
    [string]$OutputDirectory = $PSScriptRoot,
    [string]$ArchiveName,
    [switch]$UseExistingFrontend,
    [string]$CandidateDirectory,
    [string]$FrontendArtifact,
    [string]$PythonCommand,
    [switch]$Overwrite
)

$ErrorActionPreference = 'Stop'
$sourceRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$toolRoot = $sourceRoot
if ($FrontendArtifact -and (-not $CandidateDirectory -or -not $UseExistingFrontend)) {
    throw 'FrontendArtifact requires CandidateDirectory and UseExistingFrontend.'
}
function Invoke-QualityTool {
    param([string]$ToolName, [string[]]$ArgumentList)
    & $PythonCommand (Join-Path $toolRoot "tools/quality/$ToolName") @ArgumentList | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Quality tool $ToolName rejected the inputs (exit $LASTEXITCODE)."
    }
}
if ($CandidateDirectory) {
    if (-not $PythonCommand) {
        $localPython = Join-Path $toolRoot '.venv/Scripts/python.exe'
        $unixPython = Join-Path $toolRoot '.venv/bin/python'
        $PythonCommand = if (Test-Path -LiteralPath $localPython) { $localPython }
            elseif (Test-Path -LiteralPath $unixPython) { $unixPython }
            else { (Get-Command python3 -ErrorAction Stop).Source }
    }
    $CandidateDirectory = [System.IO.Path]::GetFullPath($CandidateDirectory)
    Invoke-QualityTool 'candidate.py' @('verify', '--candidate', $CandidateDirectory)
    $candidateIdentity = Get-Content -LiteralPath (Join-Path $CandidateDirectory 'candidate.json') -Raw | ConvertFrom-Json
    if ($ReleaseVersion -ne $candidateIdentity.identity.version -and $ReleaseVersion -ne "v$($candidateIdentity.identity.version)") {
        throw 'ReleaseVersion does not match candidate identity.'
    }
    $sourceRoot = Join-Path $CandidateDirectory 'source'
}
$outputRoot = [System.IO.Path]::GetFullPath($OutputDirectory)
if ($CandidateDirectory -and ($outputRoot -eq $sourceRoot -or
    $outputRoot.StartsWith($sourceRoot + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase))) {
    throw 'OutputDirectory must not modify candidate source.'
}
$safeVersion = $ReleaseVersion -replace '[^0-9A-Za-z._-]', '-'
if (-not $ArchiveName) {
    $ArchiveName = "ragflow-linux-pg-$safeVersion"
}
if ($ArchiveName -notmatch '^[0-9A-Za-z][0-9A-Za-z._-]*$') {
    throw "ArchiveName contains unsupported characters: $ArchiveName"
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

New-Item -ItemType Directory -Path $outputRoot -Force | Out-Null
$archivePath = Join-Path $outputRoot ($ArchiveName + '.tar.gz')
$checksumPath = $archivePath + '.sha256'
if ($CandidateDirectory -and (Test-Path -LiteralPath (Join-Path $CandidateDirectory 'artifact-linux-pg-source.json'))) {
    if (Test-Path -LiteralPath $archivePath) {
        Invoke-QualityTool 'candidate.py' @('verify-artifact', '--candidate', $CandidateDirectory,
            '--artifact', $archivePath, '--name', 'linux-pg-source')
    }
    throw 'Canonical candidate source archive is already recorded. Reuse its verified bytes; candidate artifacts cannot be overwritten or rebuilt at another path.'
}
if (-not $Overwrite -and ((Test-Path -LiteralPath $archivePath) -or (Test-Path -LiteralPath $checksumPath))) {
    throw "Refusing to overwrite an existing release artifact: $archivePath"
}

$tempBase = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
$tempRoot = Join-Path $tempBase ('ragflow-linux-pg-archive-' + [guid]::NewGuid().ToString('N'))
$stageRoot = Join-Path $tempRoot 'source'
$validationRoot = Join-Path $tempRoot 'validation'
New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null

try {
    # Copy the artifact into private staging before validation/extraction so a
    # concurrent change to its original path cannot replace verified tar entries.
    if ($FrontendArtifact) {
        $FrontendArtifact = [System.IO.Path]::GetFullPath($FrontendArtifact)
        Invoke-QualityTool 'frontend_artifact.py' @('verify', '--candidate', $CandidateDirectory, '--archive', $FrontendArtifact)
        $verifiedFrontend = Join-Path $tempRoot 'frontend.tar.gz'
        Copy-Item -LiteralPath $FrontendArtifact -Destination $verifiedFrontend
        Copy-Item -LiteralPath ($FrontendArtifact + '.json') -Destination ($verifiedFrontend + '.json')
        Invoke-QualityTool 'frontend_artifact.py' @('verify', '--candidate', $CandidateDirectory, '--archive', $verifiedFrontend)
    }
    $excludedDirectoryNames = @(
        '.git', '.venv', '.codex_tmp', '.playwright-cli',
        '.cache', '.hypothesis', '.mypy_cache', '.pytest_cache', '.ruff_cache',
        'node_modules', '__pycache__', 'coverage', 'ragflow-logs', 'output'
    )
    $excludedDirectoryPaths = @(
        'build', 'dist', 'release', 'web/dist',
        'services/asr-online-service/artifacts',
        'services/asr-online-service/openapi',
        'services/asr-online-service/scripts',
        'services/asr-online-service/tests',
        'services/asr-online-service/uploads',
        'test/playwright/artifacts',
        'rag/res/deepdoc',
        'ragflow_deps/huggingface.co', 'ragflow_deps/nltk_data'
    )
    $exportedFileCount = 0
    function Copy-ReleaseDirectory {
        param(
            [Parameter(Mandatory = $true)][string]$SourceDirectory,
            [Parameter(Mandatory = $true)][string]$TargetDirectory,
            [string]$RelativeDirectory = ''
        )

        foreach ($item in Get-ChildItem -LiteralPath $SourceDirectory -Force) {
            $relativePath = if ($RelativeDirectory) {
                "$RelativeDirectory/$($item.Name)"
            }
            else {
                $item.Name
            }
            $normalizedPath = $relativePath.Replace('\', '/')

            if ($item.PSIsContainer) {
                if (
                    $excludedDirectoryNames -contains $item.Name -or
                    $excludedDirectoryPaths -contains $normalizedPath -or
                    $normalizedPath -match '^deployment/linux-pg/release-[^/]+(/|$)' -or
                    $item.Name -like 'cmake-build-*' -or
                    $item.Name -like '*.egg-info'
                ) {
                    continue
                }
                if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
                    throw "Refusing to package a directory link: $normalizedPath"
                }
                Copy-ReleaseDirectory `
                    -SourceDirectory $item.FullName `
                    -TargetDirectory (Join-Path $TargetDirectory $item.Name) `
                    -RelativeDirectory $normalizedPath
                continue
            }

            if (
                $item.Name -eq '.git' -or
                $normalizedPath -match '(^|/)\.env\.local$' -or
                ($item.Name -eq '.env' -and $normalizedPath -ne 'web/.env') -or
                $normalizedPath -match '\.(tar\.gz|bundle)(\.sha256)?$' -or
                $item.Name -eq '.DS_Store' -or
                $normalizedPath -match '\.(log|dump|sqlite|sqlite3)$' -or
                $normalizedPath -in @('.coverage', 'intake-complete.png', 'review-revision-1.png') -or
                $normalizedPath -match '^agent/business_requirements/document_constructor_.*\.(png|svg|puml)$' -or
                $normalizedPath -match '^[0-9a-f]{32,64}$' -or
                $normalizedPath -match '^deployment/linux-pg/registry-images-.*\.env$' -or
                (
                    $normalizedPath -match '^ragflow_deps/' -and
                    $normalizedPath -notin @(
                        'ragflow_deps/Dockerfile',
                        'ragflow_deps/download_deps.py',
                        'ragflow_deps/download_go_deps.py',
                        'ragflow_deps/prepare_native.py',
                        'ragflow_deps/native-deps.json'
                    )
                )
            ) {
                continue
            }
            if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "Refusing to package a file link: $normalizedPath"
            }

            New-Item -ItemType Directory -Path $TargetDirectory -Force | Out-Null
            Copy-Item -LiteralPath $item.FullName -Destination (Join-Path $TargetDirectory $item.Name) -Force
            $script:exportedFileCount++
        }
    }

    Copy-ReleaseDirectory -SourceDirectory $sourceRoot -TargetDirectory $stageRoot
    if ($FrontendArtifact) {
        $frontendTarget = Join-Path $stageRoot 'web/dist'
        New-Item -ItemType Directory -Path $frontendTarget -Force | Out-Null
        Invoke-Tar -ArgumentList @('-xzf', $verifiedFrontend, '-C', $frontendTarget) `
            -ErrorMessage 'Verified frontend extraction failed.'
    }
    elseif ($UseExistingFrontend) {
        Write-Warning 'Raw UseExistingFrontend is unverified legacy input; no candidate/frontend build proof is asserted.'
        $frontendDist = Join-Path $sourceRoot 'web\dist'
        if (-not (Test-Path -LiteralPath (Join-Path $frontendDist 'index.html') -PathType Leaf)) {
            throw 'UseExistingFrontend requires web/dist/index.html.'
        }
        Copy-ReleaseDirectory `
            -SourceDirectory $frontendDist `
            -TargetDirectory (Join-Path $stageRoot 'web\dist') `
            -RelativeDirectory 'web/dist'
    }
    if ($CandidateDirectory) {
        Copy-Item -LiteralPath (Join-Path $CandidateDirectory 'candidate.json') `
            -Destination (Join-Path $stageRoot 'DEPLOYMENT-CANDIDATE.json')
    }

    foreach ($scriptPath in Get-ChildItem -LiteralPath $stageRoot -Recurse -File -Filter '*.sh') {
        $relativeScript = [System.IO.Path]::GetRelativePath($stageRoot, $scriptPath.FullName).Replace('\', '/')
        if ($FrontendArtifact -and $relativeScript.StartsWith('web/dist/')) {
            continue # Verified frontend bytes are immutable, including static text assets.
        }
        $scriptContent = [System.IO.File]::ReadAllText($scriptPath.FullName).
            Replace("`r`n", "`n").
            Replace("`r", "`n")
        [System.IO.File]::WriteAllText(
            $scriptPath.FullName,
            $scriptContent,
            [System.Text.UTF8Encoding]::new($false)
        )
    }

    $manifestPath = Join-Path $stageRoot 'DEPLOYMENT-SOURCE.env'
    $manifest = @(
        "RELEASE_VERSION=$ReleaseVersion"
        'PACKAGE_FORMAT=tar.gz'
        'SOURCE_MODE=filesystem-snapshot'
        "FRONTEND_MODE=$(if ($UseExistingFrontend) { 'prebuilt' } else { 'excluded' })"
        "PACKAGED_AT_UTC=$([DateTime]::UtcNow.ToString('o'))"
    )
    if ($CandidateDirectory) {
        $manifest += "SOURCE_ID=$($candidateIdentity.source_id)"
        $manifest += 'CANDIDATE_VALIDATION=NOT_ASSERTED'
    }
    $manifest += "FRONTEND_INTEGRITY=$(if ($FrontendArtifact) { 'verified-receipt' } else { 'NOT_ASSERTED' })"
    Write-LfText -Path $manifestPath -Lines $manifest

    $requiredPaths = @(
        'deployment/linux-pg/install.sh',
        'deployment/linux-pg/upgrade.sh',
        'deployment/linux-pg/install_gvisor.sh',
        'deployment/linux-pg/docker-compose.release.yml',
        'deployment/linux-pg/seed_admin.py',
        'deployment/linux-pg/seed_asr.py',
        'deployment/linux-pg/env.template',
        'ragflow_deps/prepare_native.py',
        'ragflow_deps/native-deps.json',
        'DEPLOYMENT-SOURCE.env'
    )
    if ($UseExistingFrontend) {
        $requiredPaths += 'web/dist/index.html'
    }
    if ($CandidateDirectory) {
        $requiredPaths += 'DEPLOYMENT-CANDIDATE.json'
    }
    foreach ($relativePath in $requiredPaths) {
        if (-not (Test-Path -LiteralPath (Join-Path $stageRoot $relativePath) -PathType Leaf)) {
            throw "Required deployment file is missing: $relativePath"
        }
    }
    $crlfScripts = @(
        Get-ChildItem -LiteralPath $stageRoot -Recurse -File -Filter '*.sh' |
            Where-Object { -not ($FrontendArtifact -and [System.IO.Path]::GetRelativePath($stageRoot, $_.FullName).Replace('\', '/').StartsWith('web/dist/')) } |
            Where-Object { [System.IO.File]::ReadAllText($_.FullName).Contains("`r") } |
            ForEach-Object { [System.IO.Path]::GetRelativePath($stageRoot, $_.FullName) }
    )
    if ($crlfScripts.Count -gt 0) {
        throw "Linux scripts contain CR characters:`n$($crlfScripts -join "`n")"
    }

    $forbiddenPaths = @(
        Get-ChildItem -LiteralPath $stageRoot -Recurse -Force -File | ForEach-Object {
            $relativePath = [System.IO.Path]::GetRelativePath($stageRoot, $_.FullName).Replace('\', '/')
            $isBundledFrontend = $UseExistingFrontend -and $relativePath -match '^web/dist/'
            if (
                $relativePath -in @('docker/.env', 'docker/.env.local') -or
                (
                    $relativePath -match '(^|/)(\.git|\.venv|\.codex_tmp|\.playwright-cli|node_modules|__pycache__|coverage|ragflow-logs|output|build|cmake-build-[^/]+|dist|release)(/|$)' -and
                    -not $isBundledFrontend
                ) -or
                $relativePath -match '^services/asr-online-service/uploads/' -or
                $relativePath -match '^services/asr-online-service/(\.venv|\.pytest_cache|artifacts|openapi|scripts|tests|uploads)(/|$)' -or
                $relativePath -match '^test/playwright/artifacts/' -or
                $relativePath -match '^rag/res/deepdoc/' -or
                $relativePath -in @('.coverage', 'intake-complete.png', 'review-revision-1.png') -or
                $relativePath -match '^agent/business_requirements/document_constructor_.*\.(png|svg|puml)$' -or
                $relativePath -match '^deployment/linux-pg/registry-images-.*\.env$' -or
                $relativePath -match '^ragflow_deps/(?!Dockerfile$|download_deps\.py$|download_go_deps\.py$|prepare_native\.py$|native-deps\.json$)' -or
                $relativePath -match '^[0-9a-f]{32,64}$' -or
                $relativePath -match '\.(tar\.gz|bundle)(\.sha256)?$'
            ) {
                $relativePath
            }
        }
    )
    if ($forbiddenPaths.Count -gt 0) {
        throw "Forbidden release files were exported:`n$($forbiddenPaths -join "`n")"
    }

    if ($CandidateDirectory) {
        Invoke-QualityTool 'candidate.py' @('verify', '--candidate', $CandidateDirectory)
        $currentIdentity = Get-Content -LiteralPath (Join-Path $CandidateDirectory 'candidate.json') -Raw | ConvertFrom-Json
        if ($currentIdentity.source_id -ne $candidateIdentity.source_id) {
            throw 'Candidate identity changed during packaging.'
        }
    }
    Invoke-Tar -ArgumentList @('-czf', $archivePath, '-C', $stageRoot, '.') `
        -ErrorMessage 'Archive creation failed.'
    Invoke-Tar -ArgumentList @('-tzf', $archivePath) `
        -ErrorMessage 'Archive integrity check failed.' `
        -DiscardOutput

    New-Item -ItemType Directory -Path $validationRoot | Out-Null
    Invoke-Tar -ArgumentList @('-xzf', $archivePath, '-C', $validationRoot) `
        -ErrorMessage 'Archive validation extraction failed.'
    foreach ($relativePath in $requiredPaths) {
        if (-not (Test-Path -LiteralPath (Join-Path $validationRoot $relativePath) -PathType Leaf)) {
            throw "Required file is absent from the archive: $relativePath"
        }
    }
    if (Test-Path -LiteralPath (Join-Path $validationRoot '.git')) {
        throw 'Archive unexpectedly contains Git metadata.'
    }

    $archiveHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    Write-LfText -Path $checksumPath -Lines @(
        "$archiveHash  $([System.IO.Path]::GetFileName($archivePath))"
    )
    if ($CandidateDirectory) {
        Invoke-QualityTool 'candidate.py' @('record-artifact', '--candidate', $CandidateDirectory,
            '--artifact', $archivePath, '--name', 'linux-pg-source')
    }

    Write-Host "Archive: $archivePath"
    Write-Host "Checksum: $checksumPath"
    Write-Host "SHA256: $archiveHash"
    Write-Host "Release version: $ReleaseVersion"
    Write-Host "Packaged files: $exportedFileCount"
}
finally {
    $resolvedTemp = [System.IO.Path]::GetFullPath($tempRoot)
    if ($resolvedTemp.StartsWith($tempBase, [System.StringComparison]::OrdinalIgnoreCase) -and
        (Split-Path -Leaf $resolvedTemp) -like 'ragflow-linux-pg-archive-*') {
        Remove-Item -LiteralPath $resolvedTemp -Recurse -Force -ErrorAction SilentlyContinue
    }
}
