# Waits for the Vite dev server to answer on 127.0.0.1:<Port> before the app
# binary starts. The Wails dev-mode startup check only retries for ~5s, which
# `npm run dev` cold start on Windows regularly exceeds — this gate removes
# that race. Exits 0 once up, 1 after -TimeoutSec without an answer.
param(
    [int]$Port = 9245,
    [int]$TimeoutSec = 60
)

$url = "http://127.0.0.1:$Port/"
$deadline = (Get-Date).AddSeconds($TimeoutSec)
Write-Host "Waiting for frontend dev server at $url (up to ${TimeoutSec}s)..."

while ((Get-Date) -lt $deadline) {
    $up = $false
    try {
        $r = Invoke-WebRequest -UseBasicParsing -Uri $url -TimeoutSec 2
        if ($r.StatusCode -eq 200) { $up = $true }
    } catch {
        # not up yet
    }
    if ($up) {
        Write-Host "Frontend dev server is up."
        exit 0
    }
    Start-Sleep -Milliseconds 500
}

Write-Host "ERROR: frontend dev server did not answer at $url within ${TimeoutSec}s."
exit 1
