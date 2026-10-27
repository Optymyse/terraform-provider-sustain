package provider

import (
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// nullOf is an object of a type with every attribute null.
func nullOf(t tftypes.Type) map[string]tftypes.Value {
	object := t.(tftypes.Object)
	fields := map[string]tftypes.Value{}
	for name, inner := range object.AttributeTypes {
		fields[name] = tftypes.NewValue(inner, nil)
	}
	return fields
}

// A small resource: an assertion chosen by its type, a configuration changed field by field, and a value that may be
// null.
var checkType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
	"id":      tftypes.String,
	"name":    tftypes.String,
	"version": tftypes.Number,
	"note":    tftypes.String,
	"config": tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"url":    tftypes.String,
		"method": tftypes.String,
		"assertions": tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{
			"type":   tftypes.String,
			"value":  tftypes.String,
			"name":   tftypes.String,
			"target": tftypes.String,
		}}},
	}},
}}

var assertion = tag{property: "type", shapes: []shape{
	{allows: map[string][]string{"type": {"keyword"}}, required: []string{"value"}, takes: []string{"value"}},
	{allows: map[string][]string{"type": {"header"}}, required: []string{"name"}, nulls: []string{"target"}, takes: []string{"name", "target"}},
}}

var check = definition{
	name:    "check",
	path:    "/api/v1/checks",
	monitor: "http",
	key:     "monitor",
	sends: []string{"name", "note", "config", "config.url", "config.method", "config.assertions",
		"config.assertions[].type", "config.assertions[].value", "config.assertions[].name", "config.assertions[].target"},
	nulls:  []string{"note"},
	merges: []string{"config"},
	tags:   []tagged{{at: "config.assertions[]", tag: &assertion}},
	schema: func() schema.Schema { return schema.Schema{} },
}

func object(t tftypes.Type, fields map[string]tftypes.Value) tftypes.Value {
	full := nullOf(t)
	for name, value := range fields {
		full[name] = value
	}
	return tftypes.NewValue(t, full)
}

func str(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }

var (
	configType    = checkType.AttributeTypes["config"].(tftypes.Object)
	assertionType = configType.AttributeTypes["assertions"].(tftypes.List).ElementType
)

func checkValue(note tftypes.Value, assertions ...tftypes.Value) tftypes.Value {
	return object(checkType, map[string]tftypes.Value{
		"id":      str("m1"),
		"name":    str("Shop"),
		"version": tftypes.NewValue(tftypes.Number, big.NewFloat(3)),
		"note":    note,
		"config": object(configType, map[string]tftypes.Value{
			"url":        str("https://shop.example.com/"),
			"method":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"assertions": tftypes.NewValue(configType.AttributeTypes["assertions"], assertions),
		}),
	})
}

func asJSON(t *testing.T, value any) string {
	t.Helper()
	text, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

func TestWhatIsSentIsTheSettingsKnownAndNullOnlyWhereSentSo(t *testing.T) {
	header := object(assertionType, map[string]tftypes.Value{"type": str("header"), "name": str("cache-control")})
	sent, err := check.sent(checkValue(tftypes.NewValue(tftypes.String, nil), header), "")
	if err != nil {
		t.Fatal(err)
	}
	// No id or version, which aren't settings; no method, unknown; the note sent as null; the header's target, which
	// its shape requires but lets be null, too.
	want := `{"config":{"assertions":[{"name":"cache-control","target":null,"type":"header"}],"url":"https://shop.example.com/"},"name":"Shop","note":null}`
	if got := asJSON(t, sent); got != want {
		t.Errorf("sent %s, want %s", got, want)
	}
}

func TestAnAnswerIsReadIntoTheResourcesTypeWhatItDoesntSayNull(t *testing.T) {
	var answer map[string]any
	decoder := json.NewDecoder(strings.NewReader(`{"id":"m1","name":"Shop","version":4,"status":"up","config":{"url":"https://shop.example.com/","assertions":[]}}`))
	decoder.UseNumber()
	if err := decoder.Decode(&answer); err != nil {
		t.Fatal(err)
	}
	value, err := valueOf(checkType, answer, "")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := fieldsOf(value)
	var version big.Float
	_ = fields["version"].As(&version)
	if version.Text('g', -1) != "4" || !fields["note"].IsNull() {
		t.Errorf("read %v", value)
	}
	if _, err := valueOf(checkType, map[string]any{"name": true}, ""); err == nil || !strings.Contains(err.Error(), "name") {
		t.Errorf("a flag for a name was read: %v", err)
	}
}

func TestAMonitorsConfigurationIsReadAsItsMade(t *testing.T) {
	answer := map[string]any{"config": map[string]any{
		"target":          map[string]any{"url": "https://shop.example.com/"},
		"request":         map[string]any{"assertions": []any{}},
		"target_origin":   "https://shop.example.com",
		"assertion_count": json.Number("0"),
		"method":          "GET",
	}}
	folded(answer, configType)
	config := answer["config"].(map[string]any)
	if config["url"] != "https://shop.example.com/" || config["assertions"] == nil || config["method"] != "GET" {
		t.Errorf("folded %v", config)
	}
}

func TestAChangeKeepsWhatWasPlannedAndTakesWhatCouldntBeKnown(t *testing.T) {
	plan := checkValue(str("as given"))
	answer := checkValue(str("as Sustain writes it"))
	answered, _ := fieldsOf(answer)
	config, _ := fieldsOf(answered["config"])
	config["method"] = str("GET")
	answered["config"] = tftypes.NewValue(configType, config)
	kept, err := applied(plan, tftypes.NewValue(checkType, answered))
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := fieldsOf(kept)
	configured, _ := fieldsOf(fields["config"])
	var note, method string
	_ = fields["note"].As(&note)
	_ = configured["method"].As(&method)
	if note != "as given" || method != "GET" {
		t.Errorf("kept the note %q and method %q", note, method)
	}
}

// asKept is a check as it's kept: nothing in it unknown.
func asKept(note tftypes.Value) tftypes.Value {
	fields, _ := fieldsOf(checkValue(note))
	config, _ := fieldsOf(fields["config"])
	config["method"] = str("GET")
	fields["config"] = tftypes.NewValue(configType, config)
	return tftypes.NewValue(checkType, fields)
}

func TestAReadKeepsWhatSustainAnswersAsBeforeAndShowsWhatChangedElsewhere(t *testing.T) {
	kept := asKept(str("AB CD"))
	// Sustain wrote the note its own way when it was made, and has answered so since.
	first := asKept(str("abcd"))
	seen := map[string]string{}
	if err := remembered(first, "", seen); err != nil {
		t.Fatal(err)
	}
	same, err := refreshed(kept, first, seen, "")
	if err != nil {
		t.Fatal(err)
	}
	if !same.Equal(kept) {
		t.Errorf("a field Sustain answered as before wasn't kept: %v", same)
	}
	// Then someone changed it elsewhere.
	changed, err := refreshed(kept, asKept(str("ef")), seen, "")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := fieldsOf(changed)
	var note string
	_ = fields["note"].As(&note)
	if note != "ef" {
		t.Errorf("a change made elsewhere wasn't shown: %q", note)
	}
}

func TestAChangeSendsWhatChangedAConfigurationFieldByFieldWithTheVersion(t *testing.T) {
	r := &sustainResource{def: &check}
	prior := object(checkType, map[string]tftypes.Value{
		"id":      str("m1"),
		"name":    str("Shop"),
		"version": tftypes.NewValue(tftypes.Number, big.NewFloat(3)),
		"note":    str("old"),
		"config": object(configType, map[string]tftypes.Value{
			"url":        str("https://shop.example.com/"),
			"method":     str("GET"),
			"assertions": tftypes.NewValue(configType.AttributeTypes["assertions"], []tftypes.Value{}),
		}),
	})
	plan := object(checkType, map[string]tftypes.Value{
		"id":      str("m1"),
		"name":    str("Shop"),
		"version": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"note":    tftypes.NewValue(tftypes.String, nil),
		"config": object(configType, map[string]tftypes.Value{
			"url":        str("https://shop.example.com/health"),
			"method":     str("GET"),
			"assertions": tftypes.NewValue(configType.AttributeTypes["assertions"], []tftypes.Value{}),
		}),
	})
	body, err := r.changes(prior, plan, plan)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"config":{"url":"https://shop.example.com/health"},"expected_version":3,"note":null}`
	if got := asJSON(t, body); got != want {
		t.Errorf("sent %s, want %s", got, want)
	}
	// Nothing planned otherwise: nothing sent.
	if body, _ := r.changes(prior, prior, prior); len(body) != 0 {
		t.Errorf("sent %v", body)
	}
}

func TestATaggedObjectIsHeldToItsTagsShape(t *testing.T) {
	keyword := object(assertionType, map[string]tftypes.Value{"type": str("keyword"), "name": str("cache-control")})
	var diags = checkDiagnostics(checkValue(str("x"), keyword))
	if len(diags) != 1 || !strings.Contains(diags[0], `With type = "keyword": name isn't one it takes; value is required.`) {
		t.Errorf("said %v", diags)
	}
	header := object(assertionType, map[string]tftypes.Value{"type": str("header"), "name": str("cache-control")})
	if diags := checkDiagnostics(checkValue(str("x"), header)); len(diags) != 0 {
		t.Errorf("said %v", diags)
	}
	// A value not known yet fits whatever shape it will.
	unknown := object(assertionType, map[string]tftypes.Value{"type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "value": str("ok")})
	if diags := checkDiagnostics(checkValue(str("x"), unknown)); len(diags) != 0 {
		t.Errorf("said %v", diags)
	}
}

func checkDiagnostics(v tftypes.Value) []string {
	var found []string
	var diags diag.Diagnostics
	check.checkTags(v, "", path.Empty(), &diags)
	for _, each := range diags {
		found = append(found, each.Summary()+": "+each.Detail())
	}
	return found
}

func TestNumbersAreSentAsWritten(t *testing.T) {
	value, err := plain(tftypes.NewValue(tftypes.Number, big.NewFloat(99.9)), tftypes.Number)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(value, json.Number("99.9")) {
		t.Errorf("sent %v", value)
	}
}

func TestAPlanThatSendsNothingIsWhatsKept(t *testing.T) {
	kept := asKept(str("as kept"))
	plan := checkValue(str("as kept"))
	fields, _ := fieldsOf(plan)
	fields["version"] = tftypes.NewValue(tftypes.Number, tftypes.UnknownValue)
	plan = tftypes.NewValue(checkType, fields)
	planned, err := known(plan, kept)
	if err != nil {
		t.Fatal(err)
	}
	if !planned.Equal(kept) {
		t.Errorf("planned %v", planned)
	}
}
