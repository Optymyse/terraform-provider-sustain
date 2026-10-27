package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A data source names something the workspace has, made elsewhere, by its id or its name, so a configuration can use
// it: a monitor or a service.

var (
	_ datasource.DataSourceWithConfigure        = (*namedSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*namedSource)(nil)
)

// named is what a data source reads: its collection, and what holds one and all of them in Sustain's answers.
type named struct {
	name  string
	path  string
	key   string
	list  string
	about string
	// typed says whether it has a type, as a monitor does.
	typed bool
}

var namedSources = []named{
	{
		name: "monitor", path: "/api/v1/monitors", key: "monitor", list: "monitors", typed: true,
		about: "A monitor the workspace has, of any type, by its id or its name: what other resources name it by.",
	},
	{
		name: "service", path: "/api/v1/services", key: "service", list: "services",
		about: "A service the workspace has, by its id or its name: what other resources name it by.",
	},
}

type namedSource struct {
	source *named
	client *client
}

func (d *namedSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.source.name
}

func (d *namedSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Optional: true, Computed: true,
			Description:         "Its id: given, or found by its name.",
			MarkdownDescription: "Its id: given, or found by its name.",
		},
		"name": schema.StringAttribute{
			Optional: true, Computed: true,
			Description:         "Its name, exactly as the workspace has it, which only it may have: given, or read by its id.",
			MarkdownDescription: "Its name, exactly as the workspace has it, which only it may have: given, or read by its id.",
		},
	}
	if d.source.typed {
		attributes["type"] = schema.StringAttribute{
			Computed:            true,
			Description:         "Its type: http, tcp, udp, dns, ping, mail, grpc, browser or heartbeat.",
			MarkdownDescription: "Its type: `http`, `tcp`, `udp`, `dns`, `ping`, `mail`, `grpc`, `browser` or `heartbeat`.",
		}
	}
	resp.Schema = schema.Schema{Description: d.source.about, MarkdownDescription: d.source.about, Attributes: attributes}
}

func (d *namedSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *namedSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client)
	if !ok {
		resp.Diagnostics.AddError("The provider wasn't configured", fmt.Sprintf("It gave a %T.", req.ProviderData))
		return
	}
	d.client = c
}

func (d *namedSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var given struct {
		ID   types.String `tfsdk:"id"`
		Name types.String `tfsdk:"name"`
	}
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &given.ID)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("name"), &given.Name)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := d.find(ctx, given.ID.ValueString(), given.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("No such %s", d.source.name), err.Error())
		return
	}
	id, _ := found["id"].(string)
	name, _ := found["name"].(string)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	if d.source.typed {
		kind, _ := found["type"].(string)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), kind)...)
	}
}

// find reads one by its id, or finds the one of its name among all of them, which come whole.
func (d *namedSource) find(ctx context.Context, id, name string) (map[string]any, error) {
	if id != "" {
		answer, err := d.client.call(ctx, http.MethodGet, d.source.path+"/"+url.PathEscape(id), nil)
		if err != nil {
			return nil, err
		}
		found, ok := answer[d.source.key].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("its answer holds no %q", d.source.key)
		}
		return found, nil
	}
	answer, err := d.client.call(ctx, http.MethodGet, d.source.path, nil)
	if err != nil {
		return nil, err
	}
	all, _ := answer[d.source.list].([]any)
	var matches []map[string]any
	for _, each := range all {
		if one, ok := each.(map[string]any); ok && one["name"] == name {
			matches = append(matches, one)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("the workspace has no %s named %q", d.source.name, name)
	case 1:
		return matches[0], nil
	}
	return nil, fmt.Errorf("the workspace has %d %ss named %q: name it by its id", len(matches), d.source.name, name)
}
