$devs = @(
  @{ id = 'dev-dh-101'; name = 'Dahua DH-P3B-PV'; ip = '192.168.31.101'; port = 80; rtsp_port = 554; onvif_port = 80; manufacturer = 'Dahua'; model = 'DH-P3B-PV'; category = 'IP_CAMERA'; status = 'ONLINE' },
  @{ id = 'dev-dh-107'; name = 'Dahua HFW1249S1'; ip = '192.168.31.107'; port = 80; rtsp_port = 554; onvif_port = 80; manufacturer = 'Dahua'; model = 'DH-IPC-HFW1249S1-A-IL'; category = 'IP_CAMERA'; status = 'ONLINE' },
  @{ id = 'dev-cp-106'; name = 'CP PLUS TA21PL3'; ip = '192.168.31.106'; port = 80; rtsp_port = 554; onvif_port = 80; manufacturer = 'CP PLUS'; model = 'CP-UNC-TA21PL3-V3'; category = 'IP_CAMERA'; status = 'ONLINE' },
  @{ id = 'dev-cp-143'; name = 'CP PLUS 4K NVR'; ip = '192.168.31.143'; port = 80; rtsp_port = 554; onvif_port = 80; manufacturer = 'CP PLUS'; model = 'CP-UNR-4K4162-V2'; category = 'NVR'; status = 'ONLINE' }
)

foreach ($d in $devs) {
  $json = $d | ConvertTo-Json
  Invoke-RestMethod -Uri 'http://127.0.0.1:8080/api/v1/devices' -Method Post -Body $json -ContentType 'application/json' | Out-Null
}

$all = Invoke-RestMethod -Uri 'http://127.0.0.1:8080/api/v1/devices'
$all | Select-Object id, name, ip, manufacturer, model | Format-Table -AutoSize
