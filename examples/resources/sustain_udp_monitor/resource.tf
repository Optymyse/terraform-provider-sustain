# An NTP server, asked the time.
resource "sustain_udp_monitor" "time" {
  name   = "Time"
  config = { host = "time.example.com", port = 123, encoding = "hex", send = "1b000000" }
}
