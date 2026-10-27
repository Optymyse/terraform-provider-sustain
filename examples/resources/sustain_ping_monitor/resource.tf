resource "sustain_ping_monitor" "router" {
  name   = "Office router"
  config = { host = "router.example.com", fail_when = "none_answered" }
}
