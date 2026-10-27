# A service made in the console, by its name, so a service made here can depend on it.
data "sustain_service" "payments" {
  name = "Payments"
}

resource "sustain_service" "checkout" {
  name           = "Checkout"
  dependency_ids = [data.sustain_service.payments.id]
}
