package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The framework checks every schema as Terraform first asks for them: write-only never computed, a default only where
// it's computed, and the rest.
func TestEverySchemaIsOneTheFrameworkTakes(t *testing.T) {
	server, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, each := range resp.Diagnostics {
		t.Errorf("%s: %s", each.Summary, each.Detail)
	}
	if len(resp.ResourceSchemas) != len(definitions) {
		t.Errorf("%d resources, where %d were defined", len(resp.ResourceSchemas), len(definitions))
	}
	for _, name := range []string{"sustain_monitor", "sustain_service"} {
		if _, ok := resp.DataSourceSchemas[name]; !ok {
			t.Errorf("no data source %s", name)
		}
	}
	for _, name := range []string{"sustain_http_monitor", "sustain_service", "sustain_alert_channel", "sustain_status_page"} {
		if _, ok := resp.ResourceSchemas[name]; !ok {
			t.Errorf("no %s", name)
		}
	}
}

func TestEveryResourceValidatesItsSchemaAsTerraformAsks(t *testing.T) {
	server, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	schemas, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for name, each := range schemas.ResourceSchemas {
		// An empty configuration: refused for what it lacks, never for the schema itself.
		config, err := tfprotov6.NewDynamicValue(each.ValueType(), tftypes.NewValue(each.ValueType(), nullOf(each.ValueType())))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := server.ValidateResourceConfig(ctx, &tfprotov6.ValidateResourceConfigRequest{TypeName: name, Config: &config})
		if err != nil {
			t.Fatal(err)
		}
		for _, diag := range resp.Diagnostics {
			if diag.Severity == tfprotov6.DiagnosticSeverityError && diag.Summary != "Missing Configuration for Required Attribute" {
				t.Errorf("%s: %s: %s", name, diag.Summary, diag.Detail)
			}
		}
	}
}
