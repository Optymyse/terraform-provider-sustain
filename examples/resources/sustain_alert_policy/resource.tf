resource "sustain_alert_channel" "hook" {
  name           = "Ops webhook"
  kind           = "webhook"
  url            = "https://hooks.example.com/sustain"
  url_wo_version = 1
}

# Every contact at once, and the channel if it's still down after five minutes.
resource "sustain_alert_policy" "shop" {
  name          = "Shop"
  channel_ids   = [sustain_alert_channel.hook.id]
  delay_seconds = 0
  escalation    = [{ after_seconds = 300, channel_ids = [sustain_alert_channel.hook.id] }]
}
