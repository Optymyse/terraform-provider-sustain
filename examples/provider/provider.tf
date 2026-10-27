terraform {
  required_providers {
    sustain = {
      source  = "optymyse/sustain"
      version = "~> 0.1"
    }
  }
}

# The workspace's address, and an API token from Settings › API tokens: here, or as SUSTAIN_URL and SUSTAIN_TOKEN.
provider "sustain" {
  url = "https://acme.sustain.watch"
}
