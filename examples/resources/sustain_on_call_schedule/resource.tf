# A week each, in turn, by London's clocks, from Monday 2 November.
resource "sustain_on_call_schedule" "ops" {
  name         = "Ops"
  time_zone    = "Europe/London"
  first_shift  = "2026-11-02T09:00"
  shift_length = 1
  shift_unit   = "week"
  user_ids     = ["44444444-4444-4444-8444-444444444444"]
}
