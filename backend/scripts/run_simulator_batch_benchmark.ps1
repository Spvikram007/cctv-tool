$targets = @('hik-127-0-0-1-18081', 'dahua-127-0-0-1-18082', 'cpplus-127-0-0-1-18083')

$prevBody = @{
  action = 'HEALTH_CHECK'
  target_ids = $targets
} | ConvertTo-Json

$prev = Invoke-RestMethod -Uri 'http://127.0.0.1:8080/api/v1/batch/preview' -Method Post -Body $prevBody -ContentType 'application/json'
Write-Host "Preview Token:" $prev.preview_token

$execBody = @{
  name = 'Simulators Batch Health Check'
  action = 'HEALTH_CHECK'
  target_ids = $targets
  preview_token = $prev.preview_token
  concurrency = 3
} | ConvertTo-Json

$job = Invoke-RestMethod -Uri 'http://127.0.0.1:8080/api/v1/batch/execute' -Method Post -Body $execBody -ContentType 'application/json'

Start-Sleep -Seconds 1
$res = Invoke-RestMethod -Uri "http://127.0.0.1:8080/api/v1/batch/jobs/$($job.id)"
$res | Select-Object id, name, status, progress, total_count, completed_count, success_count, failure_count | Format-List
$res.device_results | Select-Object device_id, device_name, status, message, duration_ms | Format-Table -AutoSize
