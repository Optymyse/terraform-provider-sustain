resource "sustain_grpc_monitor" "checkout" {
  name   = "Checkout's service"
  config = { host = "grpc.example.com", port = 443, service = "shop.Checkout" }
}
