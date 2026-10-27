# Emailed to confirm before any alert reaches it.
resource "sustain_alert_contact" "ops" {
  email = "ops@example.com"
}
