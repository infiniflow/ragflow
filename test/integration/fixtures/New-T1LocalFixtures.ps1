[CmdletBinding()]
param(
    [string]$OutputDirectory = (Join-Path $PSScriptRoot '..\..\..\.codex_tmp\t1-local-fixtures')
)

$ErrorActionPreference = 'Stop'
$outputRoot = [System.IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Force -Path $outputRoot | Out-Null

$transcript = 'привет это проверка распознавания речи сегодня хорошая погода'
$shortAudio = Join-Path $outputRoot 'synthetic-russian.wav'
$longAudio = Join-Path $outputRoot 'synthetic-russian-long.wav'
$mrzImage = Join-Path $outputRoot 'synthetic-td3-closeup.png'

Add-Type -AssemblyName System.Speech
$speaker = [System.Speech.Synthesis.SpeechSynthesizer]::new()
try {
    $voice = $speaker.GetInstalledVoices() |
        Where-Object { $_.Enabled -and $_.VoiceInfo.Culture.Name -eq 'ru-RU' } |
        Select-Object -First 1
    if ($null -eq $voice) {
        throw 'A local ru-RU SAPI voice is required to generate the deterministic ASR fixture.'
    }
    $speaker.SelectVoice($voice.VoiceInfo.Name)
    $speaker.Rate = -1
    $speaker.SetOutputToWaveFile($shortAudio)
    $speaker.Speak($transcript)
}
finally {
    $speaker.Dispose()
}

$ffmpeg = Get-Command ffmpeg -ErrorAction Stop
& $ffmpeg.Source -y -v error -stream_loop 11 -i $shortAudio -c copy $longAudio
if ($LASTEXITCODE -ne 0) {
    throw "ffmpeg failed to generate the long cancellation fixture: exit $LASTEXITCODE"
}

Add-Type -AssemblyName System.Drawing
$bitmap = [System.Drawing.Bitmap]::new(2200, 520)
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$black = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::Black)
$border = [System.Drawing.Pen]::new([System.Drawing.Color]::FromArgb(180, 180, 180), 4)
$font = [System.Drawing.Font]::new('Consolas', 58, [System.Drawing.FontStyle]::Bold, [System.Drawing.GraphicsUnit]::Pixel)
try {
    $graphics.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::SingleBitPerPixelGridFit
    $graphics.Clear([System.Drawing.Color]::White)
    $graphics.DrawRectangle($border, 12, 12, 2176, 496)
    $graphics.DrawString('P<UTOERIKSSON<<ANNA<MARIA<<<<<<<<<<<<<<<<<<<', $font, $black, 45, 120)
    $graphics.DrawString('L898902C36UTO7408122F1204159ZE184226B<<<<<10', $font, $black, 45, 260)
    $bitmap.Save($mrzImage, [System.Drawing.Imaging.ImageFormat]::Png)
}
finally {
    $font.Dispose()
    $border.Dispose()
    $black.Dispose()
    $graphics.Dispose()
    $bitmap.Dispose()
}

function Get-Artifact([string]$Path) {
    $item = Get-Item -LiteralPath $Path
    [ordered]@{
        path = $item.FullName
        bytes = $item.Length
        sha256 = (Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    }
}

[ordered]@{
    generated = $true
    transcript = $transcript
    short_audio = Get-Artifact $shortAudio
    long_audio = Get-Artifact $longAudio
    mrz_image = Get-Artifact $mrzImage
} | ConvertTo-Json -Depth 4
