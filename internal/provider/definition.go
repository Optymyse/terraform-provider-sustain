package provider

import (
	"math/big"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// definition is one resource the provider manages, as tools/schemas.mjs writes it from Sustain's description and
// tools/resources.mjs (resources_gen.go). A place in it, `at`, is its attributes' names by dots, a list's items as `[]`:
// config.assertions[].type.
type definition struct {
	// name is its name after the provider's: http_monitor, for sustain_http_monitor.
	name string
	// path is its collection in Sustain's API, made with a POST there and read, changed and deleted at its id below it.
	path string
	// monitor is a monitor's type, which making it names; none for anything else.
	monitor string
	// key is what holds it in Sustain's answers: {"monitor": {…}}.
	key string
	// extras are what making it answers beside it, once: a webhook's signing secret.
	extras []string
	// sends are where its settings are, at any depth: what of it is sent.
	sends []string
	// nulls are the settings sent as null when they're null, outside a tag's shapes, whose own say.
	nulls []string
	// changed are the settings only a change takes, sent by one straight after making it.
	changed []string
	// given are the settings kept as they're given, never read back.
	given []string
	// writeOnly are the settings sent from the configuration alone, never kept, each sent again when its
	// <name>_wo_version is raised.
	writeOnly []string
	// merges are the settings a change takes field by field, so only the fields that changed are sent.
	merges []string
	// renamed are the settings whose names Terraform keeps for its own, each by the provider's name for it: Sustain's.
	renamed map[string]string
	// tags are where a tag chooses an object's shape.
	tags []tagged
	// schema is its schema.
	schema func() schema.Schema
}

// tagged is where a tag chooses an object's shape.
type tagged struct {
	at  string
	tag *tag
}

// tag is how one of an object's properties chooses which of its shapes it is (api/tags.json).
type tag struct {
	property string
	shapes   []shape
}

// shape is one of a tagged object's shapes: what it allows of the strings that choose it, what it requires, what it
// requires but lets be null, sent as null when it isn't given, and what else it takes.
type shape struct {
	allows   map[string][]string
	required []string
	nulls    []string
	takes    []string
}

// has says whether list names at itself.
func has(list []string, at string) bool {
	for _, each := range list {
		if each == at {
			return true
		}
	}
	return false
}

// tagAt is the tag that chooses the shape of the object at at, or none.
func (d *definition) tagAt(at string) *tag {
	for _, each := range d.tags {
		if each.at == at {
			return each.tag
		}
	}
	return nil
}

// field is the name Sustain gives a setting the provider names name.
func (d *definition) field(name string) string {
	if field, ok := d.renamed[name]; ok {
		return field
	}
	return name
}

// below is the place of a field of the object at at.
func below(at, name string) string {
	if at == "" {
		return name
	}
	return at + "." + name
}

// decimal is a number written in digits as Terraform holds it: to 512 bits, so a default of 99.9 is the 99.9 a
// configuration or Sustain's answer gives, which a float's isn't.
func decimal(digits string) *big.Float {
	number, _, err := big.ParseFloat(digits, 10, 512, big.ToNearestEven)
	if err != nil {
		panic(err)
	}
	return number
}
