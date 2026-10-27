# A journey through the shop, step by step; or its Playwright code, as config.script.
resource "sustain_browser_monitor" "checkout" {
  name = "Checkout journey"
  config = {
    steps = [
      { action = "goto", url = "https://shop.example.com/" },
      { action = "click", target = { by = "role", role = "button", name = "Checkout", exact = true } },
      { action = "wait_for_load", state = "load" },
    ]
  }
}
