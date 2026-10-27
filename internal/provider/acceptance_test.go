package provider

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// The acceptance tests make, change, import and delete each resource in a workspace through Terraform itself: in a
// writable local preview of Sustain's on Enterprise's plan, never a real workspace (README.md, "Testing"). Each step
// is planned again once it's applied, which must change nothing. Run as `make acceptance`, SUSTAIN_URL the preview's
// workspace.

var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"sustain": providerserver.NewProtocol6WithError(New("test")()),
}

// accepting runs a resource's steps, in a local preview alone.
func accepting(t *testing.T, steps ...resource.TestStep) {
	t.Helper()
	if os.Getenv("TF_ACC") != "" {
		address, err := url.Parse(os.Getenv("SUSTAIN_URL"))
		if err != nil || address.Scheme != "http" || !strings.HasSuffix(address.Hostname(), ".localhost") {
			t.Fatal("SUSTAIN_URL must be a local preview's workspace, as http://acme.localhost:4188")
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		Steps:                    steps,
	})
}

func TestAccHttpMonitor(t *testing.T) {
	accepting(t,
		resource.TestStep{
			Config: `
resource "sustain_http_monitor" "shop" {
  name = "Shop, made by Terraform"
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
    ]
  }
  places_to_fail = 2
  metadata       = [{ key = "team", value = "shop" }]
}`,
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue("sustain_http_monitor.shop", tfjsonpath.New("version"), knownvalue.Int64Exact(1)),
				// Sustain's own, where nothing was given.
				statecheck.ExpectKnownValue("sustain_http_monitor.shop", tfjsonpath.New("sla_target_percent"), knownvalue.Float64Exact(99.9)),
				statecheck.ExpectKnownValue("sustain_http_monitor.shop", tfjsonpath.New("config").AtMapKey("method"), knownvalue.StringExact("GET")),
			},
		},
		resource.TestStep{
			Config: `
resource "sustain_http_monitor" "shop" {
  name = "Shop's checkout, made by Terraform"
  config = {
    url             = "https://shop.example.com/checkout"
    expected_status = { mode = "class", status_class = 2 }
  }
  places_to_fail = 2
  metadata       = [{ key = "team", value = "shop" }]
}`,
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue("sustain_http_monitor.shop", tfjsonpath.New("version"), knownvalue.Int64Exact(2)),
				statecheck.ExpectKnownValue("sustain_http_monitor.shop", tfjsonpath.New("config").AtMapKey("url"), knownvalue.StringExact("https://shop.example.com/checkout")),
			},
		},
		resource.TestStep{ResourceName: "sustain_http_monitor.shop", ImportState: true, ImportStateVerify: true},
	)
}

func TestAccTcpMonitorKeepsWhatItSendsAsWritten(t *testing.T) {
	// Sustain keeps hex in lower case without its spaces; the plan stays as written. PING, and +PONG back.
	accepting(t, resource.TestStep{Config: `
resource "sustain_tcp_monitor" "cache" {
  name   = "Cache, made by Terraform"
  config = { host = "cache.example.com", port = 6379, encoding = "hex", send = "50 49 4E 47 0D 0A", expect = "2B 50 4F 4E 47" }
}`})
}

func TestAccHeartbeatMonitor(t *testing.T) {
	accepting(t,
		resource.TestStep{
			Config: `
resource "sustain_heartbeat_monitor" "backup" {
  name           = "Nightly backup, made by Terraform"
  period_seconds = 86400
  grace_seconds  = 3600
}`,
			ConfigStateChecks: []statecheck.StateCheck{
				// Where its job calls in: heartbeat.sustain.watch's, or the preview's own.
				statecheck.ExpectKnownValue("sustain_heartbeat_monitor.backup", tfjsonpath.New("address"), knownvalue.StringRegexp(regexp.MustCompile(`^https?://heartbeat\.`))),
			},
		},
		resource.TestStep{ResourceName: "sustain_heartbeat_monitor.backup", ImportState: true, ImportStateVerify: true},
	)
}

func TestAccSecret(t *testing.T) {
	config := func(version string) string {
		return `
resource "sustain_secret" "token" {
  name             = "SHOP_TOKEN"
  value            = "made-by-terraform-` + version + `"
  value_wo_version = ` + version + `
}`
	}
	accepting(t,
		resource.TestStep{
			Config:            config("1"),
			ConfigStateChecks: []statecheck.StateCheck{statecheck.ExpectKnownValue("sustain_secret.token", tfjsonpath.New("version"), knownvalue.Int64Exact(1))},
		},
		// Its value sent again, as its version was raised.
		resource.TestStep{
			Config:            config("2"),
			ConfigStateChecks: []statecheck.StateCheck{statecheck.ExpectKnownValue("sustain_secret.token", tfjsonpath.New("version"), knownvalue.Int64Exact(2))},
		},
		resource.TestStep{
			ResourceName: "sustain_secret.token", ImportState: true, ImportStateVerify: true,
			ImportStateVerifyIgnore: []string{"value_wo_version"},
		},
	)
}

func TestAccServiceDependingOnAnother(t *testing.T) {
	accepting(t,
		resource.TestStep{Config: `
resource "sustain_http_monitor" "api" {
  name   = "API, made by Terraform"
  config = { url = "https://api.example.com/health" }
}

resource "sustain_service" "api" {
  name     = "API, made by Terraform"
  monitors = [{ monitor_id = sustain_http_monitor.api.id }]
}

resource "sustain_service" "shop" {
  name           = "Shop, made by Terraform"
  description    = "What customers buy through."
  dependency_ids = [sustain_service.api.id]
}`},
		// Its description taken away: cleared, as it may be none.
		resource.TestStep{
			Config: `
resource "sustain_http_monitor" "api" {
  name   = "API, made by Terraform"
  config = { url = "https://api.example.com/health" }
}

resource "sustain_service" "api" {
  name     = "API, made by Terraform"
  monitors = [{ monitor_id = sustain_http_monitor.api.id }]
}

resource "sustain_service" "shop" {
  name           = "Shop, made by Terraform"
  dependency_ids = [sustain_service.api.id]
}`,
			ConfigStateChecks: []statecheck.StateCheck{statecheck.ExpectKnownValue("sustain_service.shop", tfjsonpath.New("description"), knownvalue.Null())},
		},
		resource.TestStep{ResourceName: "sustain_service.shop", ImportState: true, ImportStateVerify: true},
	)
}

// An alert contact is left out: making one emails it, which a preview never sends.
func TestAccAlerting(t *testing.T) {
	accepting(t,
		resource.TestStep{Config: `
resource "sustain_alert_channel" "hook" {
  name           = "Shop's hook, made by Terraform"
  kind           = "webhook"
  url            = "https://hooks.example.com/sustain"
  url_wo_version = 1
}

resource "sustain_alert_policy" "shop" {
  name          = "Shop, made by Terraform"
  channel_ids   = [sustain_alert_channel.hook.id]
  delay_seconds = 120
}`},
		resource.TestStep{
			ResourceName: "sustain_alert_channel.hook", ImportState: true, ImportStateVerify: true,
			ImportStateVerifyIgnore: []string{"signing_secret", "url_wo_version"},
		},
		resource.TestStep{ResourceName: "sustain_alert_policy.shop", ImportState: true, ImportStateVerify: true},
	)
}

func TestAccRepeatingMaintenance(t *testing.T) {
	accepting(t,
		resource.TestStep{Config: `
resource "sustain_repeating_maintenance" "nightly" {
  title            = "Nightly deploys, made by Terraform"
  days             = ["tue", "thu"]
  start_time       = "02:00"
  duration_minutes = 30
  time_zone        = "Europe/London"
}`},
		resource.TestStep{ResourceName: "sustain_repeating_maintenance.nightly", ImportState: true, ImportStateVerify: true},
	)
}

func TestAccATagsShapeIsHeldToWhenItsPlanned(t *testing.T) {
	accepting(t, resource.TestStep{
		Config: `
resource "sustain_http_monitor" "shop" {
  name = "Shop, made by Terraform"
  config = {
    url        = "https://shop.example.com/health"
    assertions = [{ id = "has-price", type = "keyword", condition = "present", pattern = { source = "literal", value = "price" } }]
  }
}`,
		PlanOnly:    true,
		ExpectError: regexp.MustCompile(`With type = "keyword": pattern isn't one it takes; value is required`),
	})
}

// The preview's one person besides its Admin, whom its People list names.
const previewPerson = "44444444-4444-4444-8444-444444444444"

func TestAccEveryOtherMonitorType(t *testing.T) {
	accepting(t,
		resource.TestStep{Config: `
resource "sustain_dns_monitor" "dns" {
  name   = "DNS, made by Terraform"
  config = { domain = "example.com", record_type = "A" }
}

resource "sustain_ping_monitor" "ping" {
  name   = "Ping, made by Terraform"
  config = { host = "example.com" }
}

resource "sustain_udp_monitor" "udp" {
  name   = "Time, made by Terraform"
  config = { host = "time.example.com", port = 123, encoding = "hex", send = "1b000000" }
}

resource "sustain_mail_monitor" "mail" {
  name   = "Mail, made by Terraform"
  config = { host = "mail.example.com", protocol = "imap" }
}

resource "sustain_grpc_monitor" "grpc" {
  name   = "gRPC, made by Terraform"
  config = { host = "grpc.example.com", port = 443, service = "shop.Checkout" }
}

resource "sustain_browser_monitor" "checkout" {
  name = "Checkout, made by Terraform"
  config = {
    steps = [
      { action = "goto", url = "https://shop.example.com/" },
      { action = "click", target = { by = "role", role = "button", name = "Checkout", exact = true } },
    ]
  }
}`},
		resource.TestStep{ResourceName: "sustain_dns_monitor.dns", ImportState: true, ImportStateVerify: true},
		resource.TestStep{ResourceName: "sustain_udp_monitor.udp", ImportState: true, ImportStateVerify: true},
		resource.TestStep{ResourceName: "sustain_mail_monitor.mail", ImportState: true, ImportStateVerify: true},
		resource.TestStep{ResourceName: "sustain_browser_monitor.checkout", ImportState: true, ImportStateVerify: true},
	)
}

func TestAccOnCallSchedule(t *testing.T) {
	accepting(t,
		resource.TestStep{Config: `
resource "sustain_on_call_schedule" "ops" {
  name         = "Ops, made by Terraform"
  time_zone    = "Europe/London"
  first_shift  = "2026-11-02T09:00"
  shift_length = 1
  shift_unit   = "week"
  user_ids     = ["` + previewPerson + `"]
}`},
		resource.TestStep{ResourceName: "sustain_on_call_schedule.ops", ImportState: true, ImportStateVerify: true},
	)
}

func TestAccStatusPage(t *testing.T) {
	accepting(t,
		resource.TestStep{Config: `
resource "sustain_status_page" "shop" {
  name        = "Shop's status, made by Terraform"
  description = "How the shop is doing."
  colour      = "#1d4ed8"
}`},
		resource.TestStep{
			Config: `
resource "sustain_status_page" "shop" {
  name = "Shop's status, made by Terraform"
}`,
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue("sustain_status_page.shop", tfjsonpath.New("description"), knownvalue.Null()),
				statecheck.ExpectKnownValue("sustain_status_page.shop", tfjsonpath.New("colour"), knownvalue.Null()),
			},
		},
		resource.TestStep{ResourceName: "sustain_status_page.shop", ImportState: true, ImportStateVerify: true},
	)
}

func TestAccPrivateLocation(t *testing.T) {
	accepting(t,
		resource.TestStep{
			Config: `
resource "sustain_private_location" "office" {
  name = "Office, made by Terraform"
}`,
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectSensitiveValue("sustain_private_location.office", tfjsonpath.New("key")),
			},
		},
		// Its key is answered once, when it's made.
		resource.TestStep{
			ResourceName: "sustain_private_location.office", ImportState: true, ImportStateVerify: true,
			ImportStateVerifyIgnore: []string{"key"},
		},
	)
}

func TestAccClient(t *testing.T) {
	// Hooli is a company in the preview's directory at Link that isn't a client yet.
	accepting(t,
		resource.TestStep{Config: `
resource "sustain_client" "hooli" {
  company_id    = "cmp_33333333333333333333333333333333"
  support_hours = { days = ["mon", "tue", "wed", "thu", "fri"], start_time = "08:00", end_time = "18:00", time_zone = "America/Los_Angeles" }
}`},
		resource.TestStep{ResourceName: "sustain_client.hooli", ImportState: true, ImportStateVerify: true},
	)
}

func TestAccAChangeMadeElsewhereShowsInThePlan(t *testing.T) {
	var id string
	config := `
resource "sustain_http_monitor" "shop" {
  name   = "Shop, made by Terraform"
  config = { url = "https://shop.example.com/health" }
}`
	accepting(t,
		resource.TestStep{
			Config: config,
			Check: func(s *terraform.State) error {
				id = s.RootModule().Resources["sustain_http_monitor.shop"].Primary.ID
				return nil
			},
		},
		resource.TestStep{
			// Renamed in the console meanwhile: the plan names it back.
			PreConfig: func() {
				c, err := newClient(os.Getenv("SUSTAIN_URL"), os.Getenv("SUSTAIN_TOKEN"), "test")
				if err == nil {
					_, err = c.call(context.Background(), http.MethodPatch, "/api/v1/monitors/"+id,
						map[string]any{"name": "Renamed elsewhere", "expected_version": 1})
				}
				if err != nil {
					t.Fatal(err)
				}
			},
			Config: config,
			ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
				plancheck.ExpectResourceAction("sustain_http_monitor.shop", plancheck.ResourceActionUpdate),
				plancheck.ExpectKnownValue("sustain_http_monitor.shop", tfjsonpath.New("name"), knownvalue.StringExact("Shop, made by Terraform")),
			}},
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue("sustain_http_monitor.shop", tfjsonpath.New("version"), knownvalue.Int64Exact(3)),
			},
		},
	)
}

// Every example the docs show is made as it's written, but an alert contact's, which emails it.
func TestAccEveryExample(t *testing.T) {
	found, err := filepath.Glob("../../examples/resources/*/resource.tf")
	if err != nil || len(found) == 0 {
		t.Fatalf("no examples: %v", err)
	}
	for _, file := range found {
		name := filepath.Base(filepath.Dir(file))
		if name == "sustain_alert_contact" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			text, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			accepting(t, resource.TestStep{
				Config:          string(text),
				ConfigVariables: config.Variables{"shop_token": config.StringVariable("made-by-terraform")},
			})
		})
	}
}

func TestAccAMonitorAndAServiceAreFoundByName(t *testing.T) {
	accepting(t, resource.TestStep{
		Config: `
resource "sustain_http_monitor" "api" {
  name   = "API, found by Terraform"
  config = { url = "https://api.example.com/health" }
}

resource "sustain_service" "api" {
  name     = "API, found by Terraform"
  monitors = [{ monitor_id = sustain_http_monitor.api.id }]
}

data "sustain_monitor" "api" {
  name       = "API, found by Terraform"
  depends_on = [sustain_http_monitor.api]
}

data "sustain_service" "api" {
  name       = "API, found by Terraform"
  depends_on = [sustain_service.api]
}`,
		ConfigStateChecks: []statecheck.StateCheck{
			statecheck.CompareValuePairs("data.sustain_monitor.api", tfjsonpath.New("id"), "sustain_http_monitor.api", tfjsonpath.New("id"), compare.ValuesSame()),
			statecheck.ExpectKnownValue("data.sustain_monitor.api", tfjsonpath.New("type"), knownvalue.StringExact("http")),
			statecheck.CompareValuePairs("data.sustain_service.api", tfjsonpath.New("id"), "sustain_service.api", tfjsonpath.New("id"), compare.ValuesSame()),
		},
	})
}
