// What the provider manages, resource by resource, and what it makes of each beyond what Sustain's description says
// (README.md, "How it's made"). Each is its API's collection, made with a POST there and read, changed and deleted at
// its id below it; a monitor is one of the monitors' types. Read by tools/derive.mjs and tools/schemas.mjs.
//
// - leaves: left out: what happens to it, never how the workspace is set up, as it would change between plans with
//   nothing changed: its health, when it was last changed, what uses it; and what Sustain takes nothing of yet.
// - given: kept as it was given, never read back, as the API answers something else by that name.
// - writeOnly: sent, never kept, as Terraform's write-only arguments are: each sent again when its _wo_version rises.
// - sensitive: kept, never shown.
// - derived: made by Sustain from what else it's given when it's left out, so never carried from one read to the next
//   plan.
// - stable: what a change leaves as it was, so carried into the next plan; an id and when it was made always are.
// - keeps: what a monitor's answer says of it beyond what it's made with, kept as the provider read it.
// - merges: what a change takes field by field, so only its fields that changed are sent.
// - renamed: a field whose name Terraform keeps for its own, by the name the provider gives it.
// - about: what it is, as the provider's docs say it.
//
// Each names Sustain's fields, by their places in it: config.steps.

// A monitor's project is refused unless it's none, until there are projects (Sustain's step 1.12), and never answered.
// Its configuration is changed field by field, the rest kept (Sustain's docs/MONITORS_AS_CODE.md).
const monitor = (type, extra = {}) => ({
  path: "/api/v1/monitors",
  monitor: type,
  leaves: ["project_id"],
  merges: ["config"],
  ...extra,
});

export const RESOURCES = {
  http_monitor: monitor("http", {
    about: "A website or API's monitor: its address, requested from where it's checked from, and what must answer.",
  }),
  tcp_monitor: monitor("tcp", {
    about:
      "A TCP port's monitor: a connection to its host's port, secured with TLS when it asks, what it sends once " +
      "connected, and what the server must say.",
  }),
  udp_monitor: monitor("udp", {
    about:
      "A UDP port's monitor: what it sends to its host's port, sent again while no reply comes, and what the reply " +
      "must hold.",
  }),
  dns_monitor: monitor("dns", {
    about: "A name's DNS records' monitor: its records of a type, as public resolvers answer, holding what's expected.",
  }),
  ping_monitor: monitor("ping", {
    about: "A host's ping monitor: five pings from each area, over the family chosen, failing as it chooses.",
  }),
  mail_monitor: monitor("mail", {
    about:
      "A mail server's monitor: its first reply, and one fixed command answered, in SMTP, IMAP or POP3, its session " +
      "secured as it chooses.",
  }),
  grpc_monitor: monitor("grpc", {
    about:
      "A gRPC server's monitor: its health, asked by gRPC's health checking protocol of a service or the server's " +
      "own, over HTTP/2, secured with TLS when it asks.",
  }),
  // A script is read into steps, and steps given leave the script null.
  browser_monitor: monitor("browser", {
    derived: ["config.steps", "config.script"],
    about: "A browser check's monitor: its steps, or Playwright code read into them, taken in a real browser.",
  }),
  // Its address is where its job reports, which anyone holding it can: made with it, and kept.
  heartbeat_monitor: monitor("heartbeat", {
    keeps: ["address"],
    sensitive: ["address"],
    stable: ["address"],
    about:
      "A scheduled job's monitor: the job calls its address, and silence past the period and grace is an outage. " +
      "Its address is kept, sensitive, as whoever holds it can report.",
  }),
  service: {
    path: "/api/v1/services",
    leaves: ["status", "status_since", "shown_status", "affected_by", "outage", "manual_status_since", "updated_at"],
    renamed: { depends_on: "dependency_ids" },
    about: "Monitors grouped as one service, which is down by its rule and has one incident and alert for them.",
  },
  secret: {
    path: "/api/v1/secrets",
    writeOnly: ["value"],
    leaves: ["used_by", "updated_at"],
    about: "A value monitors' requests are made with that must stay private: sent, sealed, and never read back.",
  },
  // A new company is made in Link with what's given; the client's answer is Link's company as it is now.
  client: {
    path: "/api/v1/clients",
    given: ["company"],
    leaves: ["link", "service_ids", "page_id"],
    about: "A client: a company in the organisation's directory at Link, with its services and its status page.",
  },
  // Monitors added and removed by the change are its list, given whole.
  alert_policy: {
    path: "/api/v1/alert-policies",
    leaves: ["add_monitor_ids", "remove_monitor_ids", "updated_at"],
    about: "Who a monitor's or a service's alerts reach, and when.",
  },
  // A webhook's signing secret is answered once, when it's made.
  alert_channel: {
    path: "/api/v1/alert-channels",
    writeOnly: ["url", "key"],
    sensitive: ["signing_secret"],
    stable: ["signing_secret"],
    leaves: ["last_success_at", "last_failure_at", "failures", "failing_since", "turned_off_at"],
    about:
      "Somewhere alerts are sent beside email: a webhook, Slack, Discord, Teams, PagerDuty, Opsgenie or Telegram. " +
      "Its address and key are sent, never kept; a webhook's signing secret is kept, sensitive.",
  },
  alert_contact: {
    path: "/api/v1/alert-contacts",
    leaves: ["status", "confirmed_at", "suppressed"],
    about: "An address alerts are emailed to once it's confirmed: one of the workspace's people, or another address.",
  },
  // A change takes effect when it's made: one from a later time, and its overrides, stay in the console.
  on_call_schedule: {
    path: "/api/v1/on-call-schedules",
    leaves: ["changes_from", "change", "now", "policies", "updated_at"],
    about: "Some of the workspace's people, on call in turn by the clock in its time zone.",
  },
  status_page: {
    path: "/api/v1/status-pages",
    writeOnly: ["password"],
    about:
      "One of a workspace's status pages: its own, at the workspace's address, or a page of its own on a domain " +
      "named to it.",
  },
  repeating_maintenance: {
    path: "/api/v1/maintenance/repeating",
    leaves: ["next"],
    about: "Maintenance on the same days of every week, at the same time by its time zone's clocks.",
  },
  // Its key is answered once, when it's made, for its agent.
  private_location: {
    path: "/api/v1/private-locations",
    sensitive: ["key"],
    stable: ["key", "key_prefix", "key_made_at"],
    leaves: ["seen_at", "agent_version", "state"],
    about:
      "One of the workspace's own agents, run inside its own network, which checks its monitors there for it alone. " +
      "Its key is answered once, when it's made, and kept, sensitive.",
  },
};
