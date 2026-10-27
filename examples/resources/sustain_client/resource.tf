# A company in the organisation's directory at Link, by Link's id. Enterprise's.
resource "sustain_client" "hooli" {
  company_id    = "cmp_33333333333333333333333333333333"
  support_hours = { days = ["mon", "tue", "wed", "thu", "fri"], start_time = "09:00", end_time = "17:30", time_zone = "Europe/London" }
}
