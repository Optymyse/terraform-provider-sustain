resource "sustain_http_monitor" "checkout" {
  name = "Checkout"
  config = {
    url             = "https://shop.example.com/health"
    expected_status = { mode = "range", min_status_code = 200, max_status_code = 299 }
    assertions = [
      {
        id        = "no-store"
        type      = "header"
        name      = "cache-control"
        condition = "equals"
        expected  = { source = "literal", value = "no-store" }
      },
      { id = "in-stock", type = "keyword", condition = "present", value = { source = "literal", value = "In stock" } },
    ]
  }
  check_areas    = ["uk", "eu", "us"]
  places_to_fail = 2
  metadata       = [{ key = "team", value = "shop" }]
}
