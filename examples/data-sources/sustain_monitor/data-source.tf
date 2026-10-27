# A monitor made in the console, by its name, so a service made here can take it.
data "sustain_monitor" "website" {
  name = "Website"
}

resource "sustain_service" "shop" {
  name     = "Shop"
  monitors = [{ monitor_id = data.sustain_monitor.website.id }]
}
