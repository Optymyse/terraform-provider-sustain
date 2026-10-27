package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// seenKey is where a resource's private state keeps what Sustain last answered of it (values.go, remembered).
const seenKey = "answered"

var (
	_ resource.ResourceWithConfigure      = (*sustainResource)(nil)
	_ resource.ResourceWithImportState    = (*sustainResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*sustainResource)(nil)
	_ resource.ResourceWithValidateConfig = (*sustainResource)(nil)
)

// sustainResource is one of Sustain's resources, as its definition says it: made, read, changed and deleted through
// the API, alike for each.
type sustainResource struct {
	def    *definition
	client *client
}

func (r *sustainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.def.name
}

func (r *sustainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = r.def.schema()
}

func (r *sustainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client)
	if !ok {
		resp.Diagnostics.AddError("The provider wasn't configured", fmt.Sprintf("It gave a %T.", req.ProviderData))
		return
	}
	r.client = c
}

func (r *sustainResource) ValidateConfig(_ context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	r.def.checkTags(req.Config.Raw, "", path.Empty(), &resp.Diagnostics)
}

// ModifyPlan plans nothing where nothing would be sent. Terraform proposes what Sustain gave a configuration's fields
// when they weren't given as unknown, which the framework reads as a change, so plans everything computed as unknown,
// a monitor's version among them. Where no setting would change, the plan is what's kept.
func (r *sustainResource) ModifyPlan(_ context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() || req.Plan.Raw.IsFullyKnown() {
		return
	}
	changes, err := r.changes(req.State.Raw, req.Plan.Raw, req.Config.Raw)
	if err != nil {
		resp.Diagnostics.AddError("The provider couldn't plan the change", err.Error())
		return
	}
	if len(changes) > 0 {
		return
	}
	kept, err := known(req.Plan.Raw, req.State.Raw)
	if err != nil {
		resp.Diagnostics.AddError("The provider couldn't plan the change", err.Error())
		return
	}
	resp.Plan.Raw = kept
}

func (r *sustainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *sustainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	body, err := r.made(req.Plan.Raw, req.Config.Raw)
	if err != nil {
		resp.Diagnostics.AddError("The provider couldn't write the request", err.Error())
		return
	}
	answer, err := r.client.call(ctx, http.MethodPost, r.def.path, body)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Sustain didn't make the %s", r.def.name), err.Error())
		return
	}
	object, err := r.answered(answer, true)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("The %s Sustain made couldn't be read", r.def.name), err.Error())
		return
	}
	// Kept at once, so what Sustain made is known even if what follows fails: Terraform then makes it again.
	r.keep(ctx, req.Plan.Raw, object, &resp.State.Raw, resp.Private.SetKey, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	changes, err := r.afterMaking(req.Plan.Raw, req.Config.Raw, object)
	if err != nil {
		resp.Diagnostics.AddError("The provider couldn't write the request", err.Error())
		return
	}
	if len(changes) == 0 {
		return
	}
	answer, err = r.client.call(ctx, http.MethodPatch, r.itemPath(object), changes)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Sustain made the %s, but not all it was given", r.def.name), err.Error())
		return
	}
	object, err = r.answered(answer, false)
	if err == nil {
		// What making it answered once is kept from then.
		object, err = r.kept(object, resp.State.Raw)
	}
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("The %s Sustain changed couldn't be read", r.def.name), err.Error())
		return
	}
	r.keep(ctx, req.Plan.Raw, object, &resp.State.Raw, resp.Private.SetKey, &resp.Diagnostics)
}

func (r *sustainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	id, err := attribute(req.State.Raw, "id")
	if err != nil {
		resp.Diagnostics.AddError("The state has no id", err.Error())
		return
	}
	answer, err := r.client.call(ctx, http.MethodGet, r.def.path+"/"+url.PathEscape(id), nil)
	if notFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Sustain didn't answer for the %s", r.def.name), err.Error())
		return
	}
	object, err := r.answered(answer, false)
	if err == nil {
		object, err = r.kept(object, req.State.Raw)
	}
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("The %s Sustain answered couldn't be read", r.def.name), err.Error())
		return
	}
	seen := map[string]string{}
	if text, diags := req.Private.GetKey(ctx, seenKey); !diags.HasError() && len(text) > 0 {
		_ = json.Unmarshal(text, &seen)
	}
	state, err := refreshed(req.State.Raw, object, seen, "")
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("The %s Sustain answered couldn't be read", r.def.name), err.Error())
		return
	}
	resp.State.Raw = state
	r.remember(ctx, object, resp.Private.SetKey, &resp.Diagnostics)
}

func (r *sustainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	changes, err := r.changes(req.State.Raw, req.Plan.Raw, req.Config.Raw)
	if err != nil {
		resp.Diagnostics.AddError("The provider couldn't write the request", err.Error())
		return
	}
	id, err := attribute(req.State.Raw, "id")
	if err != nil {
		resp.Diagnostics.AddError("The state has no id", err.Error())
		return
	}
	item := r.def.path + "/" + url.PathEscape(id)
	var answer map[string]any
	if len(changes) > 0 {
		answer, err = r.client.call(ctx, http.MethodPatch, item, changes)
	}
	// Nothing to send, or nothing Sustain finds changed, as when it writes what's planned as it already was: read as it is.
	if len(changes) == 0 || refusedAs(err, "MONITOR_UNCHANGED") {
		answer, err = r.client.call(ctx, http.MethodGet, item, nil)
	}
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Sustain didn't change the %s", r.def.name), err.Error())
		return
	}
	object, err := r.answered(answer, false)
	if err == nil {
		object, err = r.kept(object, req.State.Raw)
	}
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("The %s Sustain changed couldn't be read", r.def.name), err.Error())
		return
	}
	r.keep(ctx, req.Plan.Raw, object, &resp.State.Raw, resp.Private.SetKey, &resp.Diagnostics)
}

func (r *sustainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	id, err := attribute(req.State.Raw, "id")
	if err != nil {
		resp.Diagnostics.AddError("The state has no id", err.Error())
		return
	}
	_, err = r.client.call(ctx, http.MethodDelete, r.def.path+"/"+url.PathEscape(id), nil)
	if err != nil && !notFound(err) {
		resp.Diagnostics.AddError(fmt.Sprintf("Sustain didn't delete the %s", r.def.name), err.Error())
	}
}

// objectType is the resource's whole value's type.
func (r *sustainResource) objectType() tftypes.Object {
	return r.def.schema().Type().TerraformType(context.Background()).(tftypes.Object)
}

// answered is the resource as Sustain answered it, as a value of its type: the object its answer holds, with what
// making it answers beside it, a monitor's configuration read as it's made, and nothing write-only.
func (r *sustainResource) answered(answer map[string]any, making bool) (tftypes.Value, error) {
	object, ok := answer[r.def.key].(map[string]any)
	if !ok {
		return tftypes.Value{}, fmt.Errorf("its answer holds no %q", r.def.key)
	}
	if making {
		for _, extra := range r.def.extras {
			object[extra] = answer[extra]
		}
	}
	whole := r.objectType()
	if r.def.monitor != "" {
		if config, ok := whole.AttributeTypes["config"].(tftypes.Object); ok {
			folded(object, config)
		}
	}
	for name, field := range r.def.renamed {
		object[name] = object[field]
		delete(object, field)
	}
	for _, name := range r.def.writeOnly {
		delete(object, name)
	}
	return valueOf(whole, object, "")
}

// folded reads a monitor's configuration as it's made: what a writer is answered beneath it, its target's address and
// its request's fields, beside the rest, in place of the summaries anyone else reads.
func folded(object map[string]any, config tftypes.Object) {
	answered, ok := object["config"].(map[string]any)
	if !ok {
		return
	}
	for name, value := range answered {
		if _, setting := config.AttributeTypes[name]; setting {
			continue
		}
		if beneath, ok := value.(map[string]any); ok {
			for inner, field := range beneath {
				if _, setting := config.AttributeTypes[inner]; setting {
					answered[inner] = field
				}
			}
		}
	}
}

// kept is an answer with what Sustain doesn't answer of the resource as it was kept: what it was given, what making it
// answered once, and when each write-only value was last sent.
func (r *sustainResource) kept(answer, prior tftypes.Value) (tftypes.Value, error) {
	if prior.IsNull() {
		return answer, nil
	}
	var fields, before map[string]tftypes.Value
	if err := answer.As(&fields); err != nil {
		return tftypes.Value{}, err
	}
	if err := prior.As(&before); err != nil {
		return tftypes.Value{}, err
	}
	names := append(append([]string{}, r.def.given...), r.def.extras...)
	for _, name := range r.def.writeOnly {
		names = append(names, name+"_wo_version")
	}
	for _, name := range names {
		if value, ok := before[name]; ok {
			fields[name] = value
		}
	}
	return tftypes.NewValue(answer.Type(), fields), nil
}

// keep sets the state after a change Sustain answered, and remembers the answer.
func (r *sustainResource) keep(ctx context.Context, plan, answer tftypes.Value, state *tftypes.Value, set func(context.Context, string, []byte) diag.Diagnostics, diags *diag.Diagnostics) {
	value, err := applied(plan, answer)
	if err != nil {
		diags.AddError(fmt.Sprintf("The %s Sustain answered couldn't be kept", r.def.name), err.Error())
		return
	}
	*state = value
	r.remember(ctx, answer, set, diags)
}

// remember keeps what Sustain answered in the resource's private state, for the next read.
func (r *sustainResource) remember(ctx context.Context, answer tftypes.Value, set func(context.Context, string, []byte) diag.Diagnostics, diags *diag.Diagnostics) {
	seen := map[string]string{}
	if err := remembered(answer, "", seen); err != nil {
		diags.AddError(fmt.Sprintf("The %s Sustain answered couldn't be remembered", r.def.name), err.Error())
		return
	}
	text, err := json.Marshal(seen)
	if err != nil {
		diags.AddError(fmt.Sprintf("The %s Sustain answered couldn't be remembered", r.def.name), err.Error())
		return
	}
	diags.Append(set(ctx, seenKey, text)...)
}

// itemPath is where a resource Sustain answered is, by its id.
func (r *sustainResource) itemPath(object tftypes.Value) string {
	id, _ := attribute(object, "id")
	return r.def.path + "/" + url.PathEscape(id)
}

// attribute is a string attribute of a resource's value.
func attribute(v tftypes.Value, name string) (string, error) {
	var fields map[string]tftypes.Value
	if err := v.As(&fields); err != nil {
		return "", err
	}
	var text string
	if err := fields[name].As(&text); err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("its %s is empty", name)
	}
	return text, nil
}
