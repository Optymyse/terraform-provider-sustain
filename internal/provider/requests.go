package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// fieldsOf is an object value's fields; none of a null one.
func fieldsOf(v tftypes.Value) (map[string]tftypes.Value, error) {
	fields := map[string]tftypes.Value{}
	if v.IsNull() || !v.IsKnown() {
		return fields, nil
	}
	err := v.As(&fields)
	return fields, err
}

// made is what making the resource is sent: its settings as planned, but those only a change takes, and each
// write-only value as configured; a monitor's type.
func (r *sustainResource) made(plan, config tftypes.Value) (map[string]any, error) {
	body, err := r.settings(plan, config, func(name string) bool { return !has(r.def.changed, name) })
	if err != nil {
		return nil, err
	}
	if r.def.monitor != "" {
		body["type"] = r.def.monitor
	}
	return body, nil
}

// afterMaking is what a change straight after making the resource is sent: the settings only a change takes, where
// they're given; nothing when none are.
func (r *sustainResource) afterMaking(plan, config, made tftypes.Value) (map[string]any, error) {
	body, err := r.settings(plan, config, func(name string) bool { return has(r.def.changed, name) })
	if err != nil {
		return nil, err
	}
	for name, value := range body {
		if value == nil {
			delete(body, name)
		}
	}
	if len(body) > 0 && r.def.monitor != "" {
		if err := r.versioned(body, made); err != nil {
			return nil, err
		}
	}
	return body, nil
}

// settings are the resource's settings of the plan that pick takes, as they're sent, each write-only one from the
// configuration.
func (r *sustainResource) settings(plan, config tftypes.Value, pick func(string) bool) (map[string]any, error) {
	planned, err := fieldsOf(plan)
	if err != nil {
		return nil, err
	}
	configured, err := fieldsOf(config)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	for name, value := range planned {
		if !has(r.def.sends, name) || !pick(name) {
			continue
		}
		if has(r.def.writeOnly, name) {
			value = configured[name]
		}
		if !value.IsKnown() || (value.IsNull() && !has(r.def.nulls, name)) {
			continue
		}
		sent, err := r.def.sent(value, name)
		if err != nil {
			return nil, err
		}
		body[r.def.field(name)] = sent
	}
	return body, nil
}

// changes are what a change is sent: each setting planned otherwise than it was kept, a configuration field by field
// where a change takes it so; each write-only value whose version was raised, from the configuration; and a monitor's
// version as it was last read, so a change made elsewhere since is refused rather than lost. Nothing when nothing
// changed.
func (r *sustainResource) changes(prior, plan, config tftypes.Value) (map[string]any, error) {
	before, err := fieldsOf(prior)
	if err != nil {
		return nil, err
	}
	planned, err := fieldsOf(plan)
	if err != nil {
		return nil, err
	}
	configured, err := fieldsOf(config)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	for name, value := range planned {
		if !has(r.def.sends, name) || has(r.def.given, name) {
			continue
		}
		if has(r.def.writeOnly, name) {
			version := name + "_wo_version"
			if planned[version].Equal(before[version]) {
				continue
			}
			value = configured[name]
			if value.IsNull() && !has(r.def.nulls, name) {
				continue
			}
		} else if !value.IsKnown() || value.Equal(before[name]) {
			continue
		}
		if has(r.def.merges, name) && !value.IsNull() && !before[name].IsNull() {
			fields, err := r.merged(before[name], value, name)
			if err != nil {
				return nil, err
			}
			if len(fields) > 0 {
				body[r.def.field(name)] = fields
			}
			continue
		}
		sent, err := r.def.sent(value, name)
		if err != nil {
			return nil, err
		}
		body[r.def.field(name)] = sent
	}
	if len(body) > 0 && r.def.monitor != "" {
		if err := r.versioned(body, prior); err != nil {
			return nil, err
		}
	}
	return body, nil
}

// merged is a change to an object Sustain takes field by field: those of its fields planned otherwise than they were.
func (r *sustainResource) merged(before, after tftypes.Value, at string) (map[string]any, error) {
	was, err := fieldsOf(before)
	if err != nil {
		return nil, err
	}
	now, err := fieldsOf(after)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{}
	for name, value := range now {
		place := below(at, name)
		if !has(r.def.sends, place) || !value.IsKnown() || value.Equal(was[name]) {
			continue
		}
		if value.IsNull() && !has(r.def.nulls, place) {
			continue
		}
		sent, err := r.def.sent(value, place)
		if err != nil {
			return nil, err
		}
		fields[name] = sent
	}
	return fields, nil
}

// versioned names the version a monitor was last read at, which a change of it must.
func (r *sustainResource) versioned(body map[string]any, kept tftypes.Value) error {
	fields, err := fieldsOf(kept)
	if err != nil {
		return err
	}
	version, ok := fields["version"]
	if !ok || !version.IsKnown() || version.IsNull() {
		return fmt.Errorf("the monitor's version isn't known")
	}
	value, err := whole(version)
	if err != nil {
		return err
	}
	body["expected_version"] = value
	return nil
}
