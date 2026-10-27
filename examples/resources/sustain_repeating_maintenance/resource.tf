# Deploys on Tuesday and Thursday nights: their failures alert nobody.
resource "sustain_repeating_maintenance" "deploys" {
  title            = "Deploys"
  days             = ["tue", "thu"]
  start_time       = "02:00"
  duration_minutes = 30
  time_zone        = "Europe/London"
  message          = "Releasing the week's changes."
}
