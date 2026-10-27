package provider

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// A tagged object is one of several shapes, chosen by its tag, as an assertion is by its type: Sustain's description
// gives each shape apart, and the provider's schema one object with every shape's fields, so the provider holds a value
// to its tag's shape, as Sustain's API would, when it's planned (api/tags.json).

// fits is what keeps an object from being a shape: nothing when it is one. Unknown fields fit any shape.
func (s *shape) fits(fields map[string]tftypes.Value, property string) []string {
	var faults []string
	for name, field := range fields {
		if name == property || !field.IsKnown() || field.IsNull() {
			continue
		}
		if choices, ok := s.allows[name]; ok {
			var chosen string
			if err := field.As(&chosen); err == nil && !has(choices, chosen) {
				faults = append(faults, fmt.Sprintf("%s = %q isn't one it takes: %s", name, chosen, strings.Join(choices, ", ")))
			}
			continue
		}
		if !has(s.takes, name) {
			faults = append(faults, fmt.Sprintf("%s isn't one it takes", name))
		}
	}
	for _, name := range s.required {
		if field, ok := fields[name]; !ok || (field.IsKnown() && field.IsNull()) {
			faults = append(faults, fmt.Sprintf("%s is required", name))
		}
	}
	sort.Strings(faults)
	return faults
}

// choices are the shapes an object's tag may choose, as its value allows: all of them while it's unknown.
func (t *tag) choices(fields map[string]tftypes.Value) []*shape {
	value, ok := fields[t.property]
	if !ok || value.IsNull() {
		return nil
	}
	var chosen string
	known := value.IsKnown() && value.As(&chosen) == nil
	var found []*shape
	for index := range t.shapes {
		if !known || has(t.shapes[index].allows[t.property], chosen) {
			found = append(found, &t.shapes[index])
		}
	}
	return found
}

// shapeOf is the shape an object at its place is, if a tag chooses it and one fits it: nil otherwise.
func (d *definition) shapeOf(at string, fields map[string]tftypes.Value) *shape {
	chooser := d.tagAt(at)
	if chooser == nil {
		return nil
	}
	for _, each := range chooser.choices(fields) {
		if len(each.fits(fields, chooser.property)) == 0 {
			return each
		}
	}
	return nil
}

// checkTags finds each tagged object of a resource's configuration that fits none of the shapes its tag chooses,
// saying why it doesn't fit the first.
func (d *definition) checkTags(v tftypes.Value, at string, where path.Path, diags *diag.Diagnostics) {
	if !v.IsKnown() || v.IsNull() {
		return
	}
	switch v.Type().(type) {
	case tftypes.Object:
		var fields map[string]tftypes.Value
		if err := v.As(&fields); err != nil {
			return
		}
		if chooser := d.tagAt(at); chooser != nil {
			if value, ok := fields[chooser.property]; ok && value.IsKnown() && !value.IsNull() {
				shapes := chooser.choices(fields)
				var faults []string
				for index, each := range shapes {
					found := each.fits(fields, chooser.property)
					if len(found) == 0 {
						faults = nil
						break
					}
					if index == 0 {
						faults = found
					}
				}
				if len(faults) > 0 {
					var chosen string
					_ = value.As(&chosen)
					diags.AddAttributeError(
						where,
						fmt.Sprintf("Not what %s = %q takes", chooser.property, chosen),
						fmt.Sprintf("With %s = %q: %s.", chooser.property, chosen, strings.Join(faults, "; ")),
					)
				}
			}
		}
		for name, field := range fields {
			d.checkTags(field, below(at, name), where.AtName(name), diags)
		}
	case tftypes.List:
		var items []tftypes.Value
		if err := v.As(&items); err != nil {
			return
		}
		for index, item := range items {
			d.checkTags(item, at+"[]", where.AtListIndex(index), diags)
		}
	}
}
