variable "hook" {
  type      = string
  sensitive = true
  ephemeral = true
  default   = "https://hooks.example.com/sustain"
}

# Its address is sent, never kept: raise url_wo_version to send it again. Its signing secret is kept, sensitive.
resource "sustain_alert_channel" "hook" {
  name           = "Ops webhook"
  kind           = "webhook"
  url            = var.hook
  url_wo_version = 1
  events         = ["incidents", "maintenance"]
}
