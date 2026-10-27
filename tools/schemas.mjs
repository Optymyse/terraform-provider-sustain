// Each resource's schema, in Go, for HashiCorp's Plugin Framework, with what the provider needs to know of it beside
// (README.md, "How it's made"), written to internal/provider/resources_gen.go. Its attributes are HashiCorp's Provider
// Code Specification's, api/provider-spec.json, which their OpenAPI generator writes from api/provider.json; what each
// may be is read from api/provider.json and Sustain's own description, and what the provider makes of each from
// tools/resources.mjs. HashiCorp's framework generator would write schemas too, but it names two nested shapes alike in
// one resource, which Go refuses, and the provider reads its values without types of their own, so it isn't run.
//
// An attribute is, beyond what the specification says of it:
// - required where making the resource requires it, but for one that may be null: optional, and sent as null;
// - optional where it may be null: null when it isn't given, and sent so, so that a change clears it;
// - optional where a tag chooses the shape it's in, as the tag's shape says what it requires;
// - otherwise optional and computed: Sustain's when it isn't given, kept from one plan to the next unless derived;
// - computed alone where only an answer has it: kept from one plan to the next if it's an id, when it was made, or
//   stable;
// - made again when it's changed, where making takes it and a change doesn't;
// - write-only, with <name>_wo_version beside it, raised to send it again; and sensitive;
// - named otherwise, where Terraform keeps its name for its own, as `depends_on`.
//
// Run as `node tools/schemas.mjs`; tested by `node --test tools/schemas.test.mjs`.

import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { nullable, pascal, resourceShapes } from "./derive.mjs";
import { RESOURCES } from "./resources.mjs";

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..");
const FRAMEWORK = "github.com/hashicorp/terraform-plugin-framework";

const isObject = (value) => typeof value === "object" && value !== null && !Array.isArray(value);

/** What Terraform keeps for its own, so a resource's attribute can't be named so. */
export const RESERVED = ["connection", "count", "depends_on", "for_each", "lifecycle", "provider", "provisioner"];
const refName = (schema) => (isObject(schema) && typeof schema.$ref === "string" ? schema.$ref.split("/").pop() : null);

/** A schema of provider.json, followed through its reference. */
const resolved = (doc, schema) => (refName(schema) === null ? schema : doc.components.schemas[refName(schema)]);

/** A list's items, followed through its reference; nothing for anything else. */
function itemsOf(doc, schema) {
  const list = resolved(doc, schema);
  return list?.type === "array" || (Array.isArray(list?.type) && list.type.includes("array")) ? list.items : undefined;
}

/** What each of the specification's kinds of attribute is in Go. */
const KINDS = {
  string: { attribute: "StringAttribute", values: "String", modifiers: "stringplanmodifier", defaults: "stringdefault" },
  int64: { attribute: "Int64Attribute", values: "Int64", modifiers: "int64planmodifier", defaults: "int64default" },
  number: { attribute: "NumberAttribute", values: "Number", modifiers: "numberplanmodifier", defaults: "numberdefault" },
  bool: { attribute: "BoolAttribute", values: "Bool", modifiers: "boolplanmodifier", defaults: "booldefault" },
  list: { attribute: "ListAttribute", values: "List", modifiers: "listplanmodifier", defaults: null },
  list_nested: { attribute: "ListNestedAttribute", values: "List", modifiers: "listplanmodifier", defaults: null },
  single_nested: { attribute: "SingleNestedAttribute", values: "Object", modifiers: "objectplanmodifier", defaults: null },
};
const ELEMENTS = { string: "types.StringType", int64: "types.Int64Type", number: "types.NumberType", bool: "types.BoolType" };
const STATIC = { string: "StaticString", int64: "StaticInt64", bool: "StaticBool", number: "StaticBigFloat" };

/** Whether `at` is `place` or within it. */
const within = (at, place) => at === place || at.startsWith(`${place}.`) || at.startsWith(`${place}[`);

/**
 * One attribute as the provider holds it, from the specification's, at `level`: the settings' shape there in
 * provider.json, the tag that chooses that shape's, if one does, and where it is, its names by dots and a list's items
 * as `[]`: `at` by Sustain's names, which tools/resources.mjs uses, and `out` by the provider's.
 */
function attributeOf(context, attribute, level) {
  const { name: field } = attribute;
  const [kind] = Object.keys(attribute).filter((key) => key !== "name");
  if (KINDS[kind] === undefined) throw new Error(`${context.name}: "${field}" is a ${kind}, which isn't written yet`);
  const spec = attribute[kind];
  const top = level.at === "";
  const rules = context.resource;
  const name = top ? (rules.renamed?.[field] ?? field) : field;
  if (top && RESERVED.includes(name))
    throw new Error(`${context.name}: "${name}" is Terraform's own, so tools/resources.mjs must rename it`);
  const at = top ? field : `${level.at}.${field}`;
  const out = top ? name : `${level.out}.${field}`;
  const property = level.settings?.properties?.[field];
  const setting = property !== undefined;
  const writeOnly = top && (rules.writeOnly ?? []).includes(field);
  const given = top && (rules.given ?? []).includes(field);
  const derived = (rules.derived ?? []).some((place) => within(at, place));
  const tagged = level.tag !== null;
  const isTag = tagged && context.tags[level.tag].property === field;
  const mayBeNull = setting && nullable(property);
  // What making gives it when it's left out, where Sustain's description says so: none, if it's null. The
  // specification's own is never read, as HashiCorp's generator writes a default of null as "" and drops a number's.
  const declared = setting ? ("default" in property ? property.default : resolved(context.doc, property)?.default) : undefined;
  const stated = declared === null ? undefined : declared;

  let mode;
  if (!setting) mode = "computed";
  else if (writeOnly || given) mode = spec.computed_optional_required === "required" ? "required" : "optional";
  else if (tagged && !isTag) mode = "optional";
  else if (spec.computed_optional_required === "required") mode = mayBeNull ? "optional" : "required";
  else if (stated !== undefined) mode = "computed_optional";
  else mode = mayBeNull ? "optional" : "computed_optional";

  if (setting) context.sends.push(out);
  if (mayBeNull && !tagged && stated === undefined) context.nulls.push(out);

  const node = {
    name,
    at: out,
    kind,
    mode,
    element: kind === "list" ? Object.keys(spec.element_type)[0] : undefined,
    description:
      name === field
        ? spec.description
        : [spec.description, `Sustain's \`${field}\`, a name Terraform keeps for its own.`].filter(Boolean).join(" "),
    sensitive: writeOnly || (rules.sensitive ?? []).includes(at) || spec.sensitive === true,
    writeOnly,
    keep:
      (mode === "computed_optional" && !derived) ||
      (mode === "computed" && (rules.stable ?? []).includes(at)) ||
      (top && mode === "computed" && (name === "id" || name === "created_at")),
    replace: top && setting && context.made.includes(field),
    validators: (spec.validators ?? []).map((validator) => {
      for (const { path } of validator.custom.imports ?? []) context.imports.add(path);
      return validator.custom.schema_definition;
    }),
    default: undefined,
    attributes: undefined,
  };
  // A list's default is learned from what Sustain answers, as anything computed is; a value's is planned.
  if (stated !== undefined && mode === "computed_optional" && STATIC[kind] !== undefined) {
    context.imports.add(`${FRAMEWORK}/resource/schema/${KINDS[kind].defaults}`);
    // A number as Terraform holds it, read from its digits: a float's 99.9 is another number.
    const value = kind === "number" ? `decimal(${JSON.stringify(String(stated))})` : JSON.stringify(stated);
    node.default = `${KINDS[kind].defaults}.${STATIC[kind]}(${value})`;
    // Null is what Terraform reads as not given, so a value that may be null is never asked for as null here.
    if (mayBeNull)
      node.description = [node.description, `${JSON.stringify(stated)} when it isn't given, as Terraform can't ask for null.`]
        .filter(Boolean)
        .join(" ");
  }
  if (kind === "single_nested" || kind === "list_nested") {
    const shape = kind === "list_nested" ? itemsOf(context.doc, property) : property;
    const items = kind === "list_nested" ? "[]" : "";
    const tag = refName(shape) !== null && refName(shape) in context.tags ? refName(shape) : null;
    if (tag !== null) context.tagged.push({ at: `${out}${items}`, tag });
    const attributes = kind === "list_nested" ? spec.nested_object.attributes : spec.attributes;
    node.attributes = attributesOf(context, attributes, {
      settings: resolved(context.doc, shape),
      tag,
      at: `${at}${items}`,
      out: `${out}${items}`,
    });
  }
  return node;
}

/** A level's attributes, by name. */
function attributesOf(context, attributes, level) {
  return attributes
    .map((attribute) => attributeOf(context, attribute, level))
    .sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
}

/** One resource as the provider holds it: its attributes, and what the provider needs to know of it beside. */
export function resourceOf(spec, doc, sustain, tags, name, resource) {
  const { made, changed, answer } = resourceShapes(sustain, resource);
  const settings = doc.components.schemas[`${pascal(name)}Settings`];
  const kept = doc.components.schemas[`${pascal(name)}Kept`];
  const context = { name, resource, doc, tags, made, imports: new Set(), sends: [], nulls: [], tagged: [] };
  const attributes = attributesOf(context, spec.schema.attributes, { settings, tag: null, at: "", out: "" });
  const named = (field) => resource.renamed?.[field] ?? field;
  for (const writeOnly of resource.writeOnly ?? []) {
    const versioned = `${writeOnly}_wo_version`;
    if (attributes.some((attribute) => attribute.name === versioned))
      throw new Error(`${name}: "${versioned}" is Sustain's own, so can't say when "${writeOnly}" is sent again`);
    attributes.push({
      name: versioned,
      at: versioned,
      kind: "int64",
      mode: "optional",
      description: `Raised to send \`${writeOnly}\` again, as Terraform keeps no write-only value to compare with.`,
      sensitive: false,
      writeOnly: false,
      keep: false,
      replace: false,
      validators: [],
    });
  }
  attributes.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
  return {
    name,
    description: resource.about ?? kept.description ?? settings.description,
    attributes,
    imports: context.imports,
    definition: {
      name,
      path: resource.path,
      monitor: resource.monitor ?? "",
      key: answer.key,
      extras: answer.extras,
      sends: context.sends,
      nulls: context.nulls,
      changed: changed.map(named),
      given: (resource.given ?? []).map(named),
      writeOnly: (resource.writeOnly ?? []).map(named),
      merges: (resource.merges ?? []).map(named),
      renamed: Object.fromEntries(Object.entries(resource.renamed ?? {}).map(([field, name]) => [name, field])),
      tags: context.tagged,
    },
  };
}

const go = (text) => JSON.stringify(text);
const strings = (list) => (list.length === 0 ? "nil" : `[]string{${list.map(go).join(", ")}}`);

/** An attribute as Go, its imports noted in `imports`. */
function goAttribute(attribute, imports) {
  const kind = KINDS[attribute.kind];
  const lines = [`schema.${kind.attribute}{`];
  if (attribute.mode === "required") lines.push("Required: true,");
  if (attribute.mode === "optional" || attribute.mode === "computed_optional") lines.push("Optional: true,");
  if (attribute.mode === "computed" || attribute.mode === "computed_optional") lines.push("Computed: true,");
  if (attribute.sensitive) lines.push("Sensitive: true,");
  if (attribute.writeOnly) lines.push("WriteOnly: true,");
  if (attribute.description)
    lines.push(`Description: ${go(attribute.description)},`, `MarkdownDescription: ${go(attribute.description)},`);
  if (attribute.kind === "list") {
    imports.add(`${FRAMEWORK}/types`);
    lines.push(`ElementType: ${ELEMENTS[attribute.element]},`);
  }
  const nested = () => attribute.attributes.map((each) => `${go(each.name)}: ${goAttribute(each, imports)},`);
  if (attribute.kind === "single_nested") lines.push("Attributes: map[string]schema.Attribute{", ...nested(), "},");
  if (attribute.kind === "list_nested")
    lines.push("NestedObject: schema.NestedAttributeObject{", "Attributes: map[string]schema.Attribute{", ...nested(), "},", "},");
  if (attribute.validators.length > 0) {
    imports.add(`${FRAMEWORK}/schema/validator`);
    lines.push(`Validators: []validator.${kind.values}{`, ...attribute.validators.map((each) => `${each},`), "},");
  }
  const modifiers = [
    ...(attribute.keep ? [`${kind.modifiers}.UseStateForUnknown()`] : []),
    ...(attribute.replace ? [`${kind.modifiers}.RequiresReplace()`] : []),
  ];
  if (modifiers.length > 0) {
    imports.add(`${FRAMEWORK}/resource/schema/planmodifier`);
    imports.add(`${FRAMEWORK}/resource/schema/${kind.modifiers}`);
    lines.push(`PlanModifiers: []planmodifier.${kind.values}{`, ...modifiers.map((each) => `${each},`), "},");
  }
  if (attribute.default !== undefined) lines.push(`Default: ${attribute.default},`);
  lines.push("}");
  return lines.join("\n");
}

/** A tag's shapes as Go. */
function goTag(name, tag) {
  const shapes = tag.shapes.map((shape) => {
    const allows = Object.entries(shape.allows).map(([property, values]) => `${go(property)}: ${strings(values)}`);
    return `{allows: map[string][]string{${allows.join(", ")}}, required: ${strings(shape.required)}, nulls: ${strings(shape.nulls)}, takes: ${strings(shape.takes)}},`;
  });
  return [
    `// ${tagName(name)} is how a ${name}'s ${tag.property} chooses its shape (api/tags.json).`,
    `var ${tagName(name)} = tag{`,
    `property: ${go(tag.property)},`,
    "shapes: []shape{",
    ...shapes,
    "},",
    "}",
  ].join("\n");
}
const tagName = (name) => `${name[0].toLowerCase()}${name.slice(1)}Tag`;
const schemaName = (name) => `${name.replace(/_(.)/g, (_, letter) => letter.toUpperCase())}Schema`;

/** internal/provider/resources_gen.go: every resource's schema and definition, and the tags they name. */
export function goSource(resources, tags) {
  const imports = new Set([`${FRAMEWORK}/resource/schema`]);
  const schemas = resources.map((resource) => {
    for (const each of resource.imports) imports.add(each);
    const attributes = resource.attributes.map((each) => `${go(each.name)}: ${goAttribute(each, imports)},`);
    return [
      `func ${schemaName(resource.name)}() schema.Schema {`,
      "return schema.Schema{",
      ...(resource.description
        ? [`Description: ${go(resource.description)},`, `MarkdownDescription: ${go(resource.description)},`]
        : []),
      "Attributes: map[string]schema.Attribute{",
      ...attributes,
      "},",
      "}",
      "}",
    ].join("\n");
  });
  const named = [...new Set(resources.flatMap((resource) => resource.definition.tags.map((each) => each.tag)))].sort();
  const definitions = resources.map(({ definition }) =>
    [
      "{",
      `name: ${go(definition.name)},`,
      `path: ${go(definition.path)},`,
      `monitor: ${go(definition.monitor)},`,
      `key: ${go(definition.key)},`,
      `extras: ${strings(definition.extras)},`,
      `sends: ${strings(definition.sends)},`,
      `nulls: ${strings(definition.nulls)},`,
      `changed: ${strings(definition.changed)},`,
      `given: ${strings(definition.given)},`,
      `writeOnly: ${strings(definition.writeOnly)},`,
      `merges: ${strings(definition.merges)},`,
      `renamed: ${Object.keys(definition.renamed).length === 0 ? "nil" : `map[string]string{${Object.entries(definition.renamed).map(([name, field]) => `${go(name)}: ${go(field)}`).join(", ")}}`},`,
      `tags: ${definition.tags.length === 0 ? "nil" : `[]tagged{${definition.tags.map((each) => `{at: ${go(each.at)}, tag: &${tagName(each.tag)}}`).join(", ")}}`},`,
      `schema: ${schemaName(definition.name)},`,
      "},",
    ].join("\n"),
  );
  return [
    "// Code generated by tools/schemas.mjs from api/provider-spec.json, api/provider.json, api/tags.json, api/sustain.json",
    "// and tools/resources.mjs. DO NOT EDIT.",
    "",
    "package provider",
    "",
    "import (",
    ...[...imports].sort().map((path) => `${go(path)}`),
    ")",
    "",
    "// definitions are the resources the provider manages, each as Sustain's description and tools/resources.mjs say it.",
    "var definitions = []definition{",
    ...definitions,
    "}",
    "",
    ...named.map((name) => `${goTag(name, tags[name])}\n`),
    ...schemas.map((each) => `${each}\n`),
  ].join("\n");
}

/** Every resource the specification has, in tools/resources.mjs's order. */
export function resourcesOf(spec, doc, sustain, tags, resources = RESOURCES) {
  return Object.entries(resources).map(([name, resource]) => {
    const found = spec.resources.find((each) => each.name === name);
    if (found === undefined) throw new Error(`The specification has no ${name}: run tools/derive.mjs, then the generator`);
    return resourceOf(found, doc, sustain, tags, name, resource);
  });
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const read = (file) => JSON.parse(readFileSync(join(ROOT, "api", file), "utf8"));
  const resources = resourcesOf(read("provider-spec.json"), read("provider.json"), read("sustain.json"), read("tags.json"));
  const out = join(ROOT, "internal", "provider", "resources_gen.go");
  writeFileSync(out, goSource(resources, read("tags.json")));
  execFileSync("gofmt", ["-w", out]);
  console.log(`internal/provider/resources_gen.go: ${resources.length} resources`);
}
