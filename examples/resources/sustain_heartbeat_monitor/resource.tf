# The job calls its address, sustain_heartbeat_monitor.backup.address, when it runs.
resource "sustain_heartbeat_monitor" "backup" {
  name           = "Nightly backup"
  period_seconds = 86400
  grace_seconds  = 3600
}
