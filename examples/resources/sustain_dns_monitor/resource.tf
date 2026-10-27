resource "sustain_dns_monitor" "mail" {
  name = "Mail's records"
  config = {
    domain          = "example.com"
    record_type     = "MX"
    expected_values = ["mail.example.com"]
    match           = "contains"
  }
}
