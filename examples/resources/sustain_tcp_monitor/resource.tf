# A Redis server: PING sent once connected, and +PONG expected back.
resource "sustain_tcp_monitor" "cache" {
  name = "Cache"
  config = {
    host     = "cache.example.com"
    port     = 6379
    encoding = "hex"
    send     = "50 49 4E 47 0D 0A"
    expect   = "2B 50 4F 4E 47"
  }
}
