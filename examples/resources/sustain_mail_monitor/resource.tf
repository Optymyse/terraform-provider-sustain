resource "sustain_mail_monitor" "imap" {
  name   = "Mail"
  config = { host = "mail.example.com", protocol = "imap", security = "tls" }
}
