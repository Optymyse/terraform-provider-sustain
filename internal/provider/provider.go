// Package provider is Sustain's provider for Terraform and OpenTofu: a workspace's monitors, services, alerting and
// status pages, managed through its API (README.md).
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = (*sustainProvider)(nil)

// sustainProvider is the provider, of its release's version.
type sustainProvider struct {
	version string
}

// New makes the provider of a release's version, as Terraform starts it.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &sustainProvider{version: version} }
}

func (p *sustainProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "sustain"
	resp.Version = p.version
}

func (p *sustainProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Sustain's provider: a workspace's monitors, services, alerting and status pages, managed through its API.",
		MarkdownDescription: "Sustain's provider: a workspace's monitors, services, alerting and status pages, managed through its API.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Optional:            true,
				Description:         "The workspace's address, as https://acme.sustain.watch; SUSTAIN_URL when it isn't given.",
				MarkdownDescription: "The workspace's address, as `https://acme.sustain.watch`; `SUSTAIN_URL` when it isn't given.",
			},
			"token": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Description: "An API token, from Settings › API tokens: of scope write for monitors, services, secrets and " +
					"maintenance, admin for alerting, clients and status pages too; SUSTAIN_TOKEN when it isn't given.",
				MarkdownDescription: "An API token, from **Settings › API tokens**: of scope `write` for monitors, services, " +
					"secrets and maintenance, `admin` for alerting, clients and status pages too; `SUSTAIN_TOKEN` when it isn't given.",
			},
		},
	}
}

func (p *sustainProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config struct {
		URL   types.String `tfsdk:"url"`
		Token types.String `tfsdk:"token"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.URL.IsUnknown() || config.Token.IsUnknown() {
		// Known once what it's given is made: Terraform asks again then.
		return
	}
	address, token := os.Getenv("SUSTAIN_URL"), os.Getenv("SUSTAIN_TOKEN")
	if !config.URL.IsNull() {
		address = config.URL.ValueString()
	}
	if !config.Token.IsNull() {
		token = config.Token.ValueString()
	}
	if address == "" {
		resp.Diagnostics.AddError("No workspace", "Give the provider its workspace's address, as url or SUSTAIN_URL.")
	}
	if token == "" {
		resp.Diagnostics.AddError("No API token", "Give the provider an API token, as token or SUSTAIN_TOKEN.")
	}
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := newClient(address, token, p.version)
	if err != nil {
		resp.Diagnostics.AddError("Not a workspace's address", err.Error())
		return
	}
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *sustainProvider) Resources(_ context.Context) []func() resource.Resource {
	resources := make([]func() resource.Resource, 0, len(definitions))
	for index := range definitions {
		def := &definitions[index]
		resources = append(resources, func() resource.Resource { return &sustainResource{def: def} })
	}
	return resources
}

func (p *sustainProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	sources := make([]func() datasource.DataSource, 0, len(namedSources))
	for index := range namedSources {
		source := &namedSources[index]
		sources = append(sources, func() datasource.DataSource { return &namedSource{source: source} })
	}
	return sources
}
