package provider

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// A value's place, as a definition names it: its attributes' names by dots, a list's items as [].

// sent is a value of the plan or configuration as Sustain's API takes it, at its place: of an object, its settings
// alone, nothing unknown, and those that are null left out, but for those sent as null.
func (d *definition) sent(v tftypes.Value, at string) (any, error) {
	if v.IsNull() {
		return nil, nil
	}
	switch t := v.Type().(type) {
	case tftypes.Object:
		var fields map[string]tftypes.Value
		if err := v.As(&fields); err != nil {
			return nil, err
		}
		chosen := d.shapeOf(at, fields)
		out := map[string]any{}
		for name, field := range fields {
			place := below(at, name)
			if !has(d.sends, place) || !field.IsKnown() {
				continue
			}
			if field.IsNull() {
				if (chosen != nil && has(chosen.nulls, name)) || (chosen == nil && has(d.nulls, place)) {
					out[name] = nil
				}
				continue
			}
			value, err := d.sent(field, place)
			if err != nil {
				return nil, err
			}
			out[name] = value
		}
		return out, nil
	case tftypes.List:
		var items []tftypes.Value
		if err := v.As(&items); err != nil {
			return nil, err
		}
		out := make([]any, 0, len(items))
		for _, item := range items {
			value, err := d.sent(item, at+"[]")
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	default:
		return plain(v, t)
	}
}

// plain is a string, number or flag as JSON.
func plain(v tftypes.Value, t tftypes.Type) (any, error) {
	switch {
	case t.Is(tftypes.String):
		var s string
		err := v.As(&s)
		return s, err
	case t.Is(tftypes.Number):
		var n big.Float
		if err := v.As(&n); err != nil {
			return nil, err
		}
		return json.Number(n.Text('g', -1)), nil
	case t.Is(tftypes.Bool):
		var b bool
		err := v.As(&b)
		return b, err
	}
	return nil, fmt.Errorf("a %s isn't a value the provider sends", t)
}

// whole is any value as JSON, every attribute of an object named, null or not: what's compared and remembered of it.
func whole(v tftypes.Value) (any, error) {
	if !v.IsKnown() {
		return nil, fmt.Errorf("an unknown value can't be compared")
	}
	if v.IsNull() {
		return nil, nil
	}
	switch t := v.Type().(type) {
	case tftypes.Object:
		var fields map[string]tftypes.Value
		if err := v.As(&fields); err != nil {
			return nil, err
		}
		out := map[string]any{}
		for name, field := range fields {
			value, err := whole(field)
			if err != nil {
				return nil, err
			}
			out[name] = value
		}
		return out, nil
	case tftypes.List:
		var items []tftypes.Value
		if err := v.As(&items); err != nil {
			return nil, err
		}
		out := make([]any, 0, len(items))
		for _, item := range items {
			value, err := whole(item)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	default:
		return plain(v, t)
	}
}

// valueOf is what Sustain answered, as a value of type t: whatever it doesn't say, null.
func valueOf(t tftypes.Type, answer any, at string) (tftypes.Value, error) {
	if answer == nil {
		return tftypes.NewValue(t, nil), nil
	}
	refused := func() (tftypes.Value, error) {
		return tftypes.Value{}, fmt.Errorf("Sustain answered %s as %T, where a %s was expected", place(at), answer, t)
	}
	switch t := t.(type) {
	case tftypes.Object:
		fields, ok := answer.(map[string]any)
		if !ok {
			return refused()
		}
		values := make(map[string]tftypes.Value, len(t.AttributeTypes))
		for name, inner := range t.AttributeTypes {
			value, err := valueOf(inner, fields[name], below(at, name))
			if err != nil {
				return tftypes.Value{}, err
			}
			values[name] = value
		}
		return tftypes.NewValue(t, values), nil
	case tftypes.List:
		items, ok := answer.([]any)
		if !ok {
			return refused()
		}
		values := make([]tftypes.Value, 0, len(items))
		for _, item := range items {
			value, err := valueOf(t.ElementType, item, at+"[]")
			if err != nil {
				return tftypes.Value{}, err
			}
			values = append(values, value)
		}
		return tftypes.NewValue(t, values), nil
	}
	switch {
	case t.Is(tftypes.String):
		if s, ok := answer.(string); ok {
			return tftypes.NewValue(t, s), nil
		}
	case t.Is(tftypes.Number):
		if n, ok := answer.(json.Number); ok {
			f, _, err := big.ParseFloat(string(n), 10, 512, big.ToNearestEven)
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("Sustain answered %s as %q, which isn't a number: %w", place(at), n, err)
			}
			return tftypes.NewValue(t, f), nil
		}
	case t.Is(tftypes.Bool):
		if b, ok := answer.(bool); ok {
			return tftypes.NewValue(t, b), nil
		}
	}
	return refused()
}

// place is where a value is, as a person reads it.
func place(at string) string {
	if at == "" {
		return "the resource"
	}
	return at
}

// remembered is what the provider keeps of an answer, between reads, to tell a change made since from Sustain writing a
// value its own way: each field's digest, by its place. A field is an object's attribute, an object within it read
// field by field too; anything else is read whole.
func remembered(answer tftypes.Value, at string, into map[string]string) error {
	if object, ok := answer.Type().(tftypes.Object); ok && answer.IsKnown() && !answer.IsNull() {
		var fields map[string]tftypes.Value
		if err := answer.As(&fields); err != nil {
			return err
		}
		for name := range object.AttributeTypes {
			if err := remembered(fields[name], below(at, name), into); err != nil {
				return err
			}
		}
		return nil
	}
	digest, err := digestOf(answer)
	if err != nil {
		return err
	}
	into[at] = digest
	return nil
}

// digestOf is a short digest of a value, as remembered.
func digestOf(v tftypes.Value) (string, error) {
	value, err := whole(v)
	if err != nil {
		return "", err
	}
	text, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(text)
	return base64.RawURLEncoding.EncodeToString(sum[:12]), nil
}

// applied is what's kept after a change Sustain answered: the plan where it's known, which Sustain took as given,
// perhaps writing it its own way; what Sustain answered where the plan couldn't know.
func applied(plan, answer tftypes.Value) (tftypes.Value, error) {
	if plan.IsFullyKnown() {
		return plan, nil
	}
	if !plan.IsKnown() {
		return answer, nil
	}
	switch t := plan.Type().(type) {
	case tftypes.Object:
		var planned, answered map[string]tftypes.Value
		if err := plan.As(&planned); err != nil {
			return tftypes.Value{}, err
		}
		if !answer.IsNull() {
			if err := answer.As(&answered); err != nil {
				return tftypes.Value{}, err
			}
		}
		values := make(map[string]tftypes.Value, len(planned))
		for name, inner := range t.AttributeTypes {
			other, ok := answered[name]
			if !ok {
				other = tftypes.NewValue(inner, nil)
			}
			value, err := applied(planned[name], other)
			if err != nil {
				return tftypes.Value{}, err
			}
			values[name] = value
		}
		return tftypes.NewValue(t, values), nil
	case tftypes.List:
		var planned, answered []tftypes.Value
		if err := plan.As(&planned); err != nil {
			return tftypes.Value{}, err
		}
		if !answer.IsNull() {
			if err := answer.As(&answered); err != nil {
				return tftypes.Value{}, err
			}
		}
		// Items planned that Sustain answered in another number can't be told apart: its answer stands.
		if len(planned) != len(answered) {
			return answer, nil
		}
		values := make([]tftypes.Value, len(planned))
		for index := range planned {
			value, err := applied(planned[index], answered[index])
			if err != nil {
				return tftypes.Value{}, err
			}
			values[index] = value
		}
		return tftypes.NewValue(t, values), nil
	}
	return answer, nil
}

// refreshed is what's kept after a read: each field as it was kept, while Sustain answers it as it did when it was
// last read, however it writes it; what Sustain answers, where that's changed since, as someone changed it elsewhere.
func refreshed(prior, answer tftypes.Value, seen map[string]string, at string) (tftypes.Value, error) {
	if object, ok := answer.Type().(tftypes.Object); ok && !prior.IsNull() && !answer.IsNull() {
		var kept, answered map[string]tftypes.Value
		if err := prior.As(&kept); err != nil {
			return tftypes.Value{}, err
		}
		if err := answer.As(&answered); err != nil {
			return tftypes.Value{}, err
		}
		values := make(map[string]tftypes.Value, len(answered))
		for name := range object.AttributeTypes {
			value, err := refreshed(kept[name], answered[name], seen, below(at, name))
			if err != nil {
				return tftypes.Value{}, err
			}
			values[name] = value
		}
		return tftypes.NewValue(object, values), nil
	}
	digest, err := digestOf(answer)
	if err != nil {
		return tftypes.Value{}, err
	}
	if before, ok := seen[at]; ok && before == digest && prior.IsKnown() {
		return prior, nil
	}
	return answer, nil
}

// known is a plan with what it doesn't know taken from what's kept, as it would be with nothing changed.
func known(plan, kept tftypes.Value) (tftypes.Value, error) {
	if !plan.IsKnown() {
		return kept, nil
	}
	if plan.IsFullyKnown() || plan.IsNull() || kept.IsNull() {
		return plan, nil
	}
	switch t := plan.Type().(type) {
	case tftypes.Object:
		var planned, before map[string]tftypes.Value
		if err := plan.As(&planned); err != nil {
			return tftypes.Value{}, err
		}
		if err := kept.As(&before); err != nil {
			return tftypes.Value{}, err
		}
		values := make(map[string]tftypes.Value, len(planned))
		for name := range t.AttributeTypes {
			value, err := known(planned[name], before[name])
			if err != nil {
				return tftypes.Value{}, err
			}
			values[name] = value
		}
		return tftypes.NewValue(t, values), nil
	case tftypes.List:
		var planned, before []tftypes.Value
		if err := plan.As(&planned); err != nil {
			return tftypes.Value{}, err
		}
		if err := kept.As(&before); err != nil {
			return tftypes.Value{}, err
		}
		if len(planned) != len(before) {
			return plan, nil
		}
		values := make([]tftypes.Value, len(planned))
		for index := range planned {
			value, err := known(planned[index], before[index])
			if err != nil {
				return tftypes.Value{}, err
			}
			values[index] = value
		}
		return tftypes.NewValue(t, values), nil
	}
	return plan, nil
}
