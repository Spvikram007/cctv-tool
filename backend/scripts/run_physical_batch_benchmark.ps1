# CCTV SENTINEL UNIVERSAL — Physical Device Batch Benchmark
Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host " CCTV SENTINEL UNIVERSAL — PHASE 5 PHYSICAL BATCH BENCHMARK" -ForegroundColor Cyan
Write-Host " Target Physical Network: 192.168.31.23/24" -ForegroundColor Cyan
Write-Host " Operations: Staged Preview -> Capability Matrix -> Batch Execution" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host ""

$targetIds = @(
  'dev-192-168-31-101', 'dev-192-168-31-102', 'dev-192-168-31-103',
  'dev-192-168-31-104', 'dev-192-168-31-106', 'dev-192-168-31-107',
  'dev-192-168-31-110', 'dev-192-168-31-143'
)

# Step 1: Request Staged Batch Preview for HEALTH_CHECK
Write-Host "[1/3] Generating Staged Batch Preview for HEALTH_CHECK..." -ForegroundColor Yellow
$previewReq = @{
  action = "HEALTH_CHECK"
  target_ids = $targetIds
  payload = @{ executed_by = "Antigravity Engineer" }
} | ConvertTo-Json

$previewRes = Invoke-RestMethod -Uri "http://127.0.0.1:8080/api/v1/batch/preview" -Method Post -Body $previewReq -ContentType "application/json"
Write-Host "  -> Preview Token: $($previewRes.preview_token)" -ForegroundColor Green
Write-Host "  -> Total Targets: $($previewRes.total_targets)" -ForegroundColor Green
Write-Host "  -> Eligible Targets: $($previewRes.eligible_count)" -ForegroundColor Green
Write-Host "  -> Ineligible Targets: $($previewRes.ineligible_count)" -ForegroundColor Green
Write-Host "  -> Reboot Required: $($previewRes.reboot_required)" -ForegroundColor Green

# Step 2: Execute Batch Job with Preview Token
Write-Host ""
Write-Host "[2/3] Executing Batch Dispatch with Concurrency=4..." -ForegroundColor Yellow
$execReq = @{
  name = "Physical Fleet Live Health Audit"
  action = "HEALTH_CHECK"
  target_ids = $targetIds
  preview_token = $previewRes.preview_token
  concurrency = 4
  rate_limit = 5
  payload = @{ executed_by = "Antigravity Engineer" }
} | ConvertTo-Json

$execRes = Invoke-RestMethod -Uri "http://127.0.0.1:8080/api/v1/batch/execute" -Method Post -Body $execReq -ContentType "application/json"
$jobId = $execRes.id
Write-Host "  -> Dispatched Job ID: $jobId, Status: $($execRes.status)" -ForegroundColor Green

# Step 3: Poll Job Until Completion
Write-Host ""
Write-Host "[3/3] Polling Job Completion and Individual Device Metrics..." -ForegroundColor Yellow
$maxAttempts = 20
$attempt = 0
$finalJob = $null

while ($attempt -lt $maxAttempts) {
  Start-Sleep -Seconds 1
  $jobStatus = Invoke-RestMethod -Uri "http://127.0.0.1:8080/api/v1/batch/jobs/$jobId"
  Write-Host "  [Poll #$($attempt+1)] Status: $($jobStatus.status), Progress: $($jobStatus.progress)%, Succeeded: $($jobStatus.succeeded), Failed: $($jobStatus.failed)" -ForegroundColor Gray
  if ($jobStatus.status -eq "COMPLETED" -or $jobStatus.status -eq "FAILED" -or $jobStatus.status -eq "CANCELLED") {
    $finalJob = $jobStatus
    break
  }
  $attempt++
}

Write-Host ""
Write-Host "================== PHYSICAL BATCH BENCHMARK RESULTS ==================" -ForegroundColor Cyan
Write-Host "Job Name:    $($finalJob.name)"
Write-Host "Job Action:  $($finalJob.action)"
Write-Host "Job Status:  $($finalJob.status)"
Write-Host "Total:       $($finalJob.total)"
Write-Host "Succeeded:   $($finalJob.succeeded)"
Write-Host "Failed:      $($finalJob.failed)"
Write-Host "Skipped:     $($finalJob.skipped)"

Write-Host ""
Write-Host "--- Per-Device Execution Breakdown ---" -ForegroundColor Yellow
$finalJob.results | Select-Object device_id, ip, status, duration_ms, details | Format-Table -AutoSize

# Step 4: Verify Cryptographic Audit Chain
Write-Host ""
Write-Host "--- Cryptographic Audit Chain Verification ---" -ForegroundColor Yellow
$auditVerify = Invoke-RestMethod -Uri "http://127.0.0.1:8080/api/v1/audit/verify"
Write-Host "  -> Chain Status: $($auditVerify.status)" -ForegroundColor Green
Write-Host "  -> Algorithm:    $($auditVerify.algorithm)" -ForegroundColor Green
Write-Host "  -> Valid:        $($auditVerify.valid)" -ForegroundColor Green
Write-Host "  -> Entries:      $($auditVerify.verified_entries)" -ForegroundColor Green
