resource "sustain_http_monitor" "api" {
  name   = "API"
  config = { url = "https://api.example.com/health" }
}

resource "sustain_service" "api" {
  name     = "API"
  monitors = [{ monitor_id = sustain_http_monitor.api.id, critical = true }]
}

resource "sustain_service" "shop" {
  name           = "Shop"
  description    = "What customers buy through."
  dependency_ids = [sustain_service.api.id]
}
