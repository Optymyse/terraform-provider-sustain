variable "shop_token" {
  type      = string
  sensitive = true
  ephemeral = true
}

# Sent to Sustain, never kept in the plan or state: raise value_wo_version to send it again.
resource "sustain_secret" "shop_token" {
  name             = "SHOP_TOKEN"
  value            = var.shop_token
  value_wo_version = 1
}
