// The provider's copy of Sustain's API description, as HashiCorp's OpenAPI generator can read it (README.md, "How
// it's made"). Sustain's description is OpenAPI 3.1, read from a workspace's /api/v1/openapi.json and kept here as
// api/sustain.json; the copy, api/provider.json, is made from it by five general rules, each applied wherever it fits,
// so a new endpoint or field follows with no change here:
//
// 1. A union of literals, as `anyOf: [{const: 1}, {const: 2}]`, is one value from a list: `{type, enum}`.
// 2. A value or null, as `anyOf: [X, {type: "null"}]`, is the value: optional, it's null when it isn't given. It's
//    marked `x-nullable`, as the provider sends null for it when it's required and isn't given.
// 3. Shapes chosen by a tag, a property each gives its own choices of string (`type`, `mode`, `source`, `action`, …),
//    are one shape: the tag, with all its values listed, and every shape's other properties, optional. Each shape is
//    written beside it, to api/tags.json, as what it allows, requires, requires but may be null, and takes, for the
//    provider to hold a value to one of them at plan, as Sustain's API would.
// 4. An answer that wraps its object, as `{"service": {…}}`, is the object.
// 5. Each resource tools/resources.mjs names has paths of its own, /provider/<resource>, taking what it's made and
//    changed with, as one shape, and answering what the provider keeps of it: a monitor as it was made, with its id
//    and version; anything else as it's read, with what its making answered beside it. These paths are never called:
//    the provider calls the API's own. api/generator.yml, which the generator reads, names them.
//
// Run as `node tools/derive.mjs`; tested by `node --test tools/derive.test.mjs`.

import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { RESOURCES } from "./resources.mjs";

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..");

const isObject = (value) => typeof value === "object" && value !== null && !Array.isArray(value);
const refName = (ref) => ref.slice("#/components/schemas/".length);
const members = (schema) => schema.oneOf ?? schema.anyOf;

/** The shape a schema names, followed through its reference. */
function resolve(doc, schema) {
  return isObject(schema) && typeof schema.$ref === "string" ? doc.components.schemas[refName(schema.$ref)] : schema;
}

/** Rule 1: a union of one type's literals as that type's list of values; null when it isn't one. */
export function literalUnion(schema) {
  const choices = members(schema);
  if (!Array.isArray(choices) || choices.length === 0) return null;
  if (!choices.every((choice) => isObject(choice) && "const" in choice && typeof choice.type === "string")) return null;
  const type = choices[0].type;
  if (!choices.every((choice) => choice.type === type)) return null;
  const { oneOf: _o, anyOf: _a, ...rest } = schema;
  return { ...rest, type, enum: choices.map((choice) => choice.const) };
}

/** Rule 2: a value or null as the value, marked as one that may be null; null when it isn't one. */
export function nullablePair(schema) {
  const choices = members(schema);
  if (!Array.isArray(choices) || choices.length !== 2) return null;
  const others = choices.filter((choice) => !(isObject(choice) && choice.type === "null"));
  if (others.length !== 1) return null;
  const { oneOf: _o, anyOf: _a, ...rest } = schema;
  // A description beside the pair is the value's own, where the value has none, and so is what it is when it's left out.
  return {
    ...others[0],
    ...(rest.description && !others[0].description ? { description: rest.description } : {}),
    ...("default" in rest ? { default: rest.default } : {}),
    "x-nullable": true,
  };
}

/** Whether a value may be null: marked so by rule 2, or a type that lists null. */
export const nullable = (schema) =>
  isObject(schema) && (schema["x-nullable"] === true || (Array.isArray(schema.type) && schema.type.includes("null")));

/** A string's choices as a list: its `const`, or its `enum`; null for anything else. */
function stringChoices(property) {
  if (!isObject(property) || property.type !== "string") return null;
  if ("const" in property) return [property.const];
  return Array.isArray(property.enum) ? property.enum : null;
}

/**
 * Rule 3: shapes chosen by a tag as one shape, and each shape as it was; null when they aren't. The tag is the first
 * property every shape gives choices of string, a `const` or an `enum`. A property the shapes give different choices of string is one
 * string, from all of them; any other property must be the same thing in every shape that has it, but for its words
 * and whether it may be null, or the shapes are left as they are, with why: a monitor's nine are, which rule 5 reads
 * instead. Each shape is kept as what it allows of those strings, what it requires, what it requires but may be null,
 * and what it takes, for the provider to hold a value to one of them.
 */
export function taggedUnion(doc, schema) {
  const choices = members(schema);
  if (!Array.isArray(choices) || choices.length < 2) return null;
  const shapes = choices.map((choice) => resolve(doc, choice));
  if (!shapes.every((shape) => isObject(shape) && shape.type === "object" && isObject(shape.properties))) return null;
  const tag = Object.keys(shapes[0].properties).find((name) =>
    shapes.every((shape) => stringChoices(shape.properties[name]) !== null),
  );
  if (tag === undefined) return null;
  const properties = {};
  const names = [...new Set(shapes.flatMap((shape) => Object.keys(shape.properties)))];
  // A string whose choices differ between shapes is one string, its choices kept in each shape.
  const chosen = new Set();
  for (const name of names) {
    const forms = shapes.filter((shape) => name in shape.properties).map((shape) => shape.properties[name]);
    // Two forms that differ only in their words, or in whether they may be null, are one thing, with the first words
    // given, which may be null where any may.
    const unworded = ({ description: _d, "x-nullable": _n, ...form }) => JSON.stringify(form);
    if (forms.every((form) => unworded(form) === unworded(forms[0]))) {
      const description = forms.find((form) => form.description)?.description;
      const { "x-nullable": _n, ...first } = forms[0];
      properties[name] = {
        ...first,
        ...(description ? { description } : {}),
        ...(forms.some(nullable) ? { "x-nullable": true } : {}),
      };
      continue;
    }
    const lists = forms.map(stringChoices);
    if (lists.some((list) => list === null)) return { conflict: `"${name}" is a different thing in two of its shapes` };
    const values = [...new Set(lists.flat())];
    const description = forms.find((form) => form.description)?.description;
    properties[name] = { type: "string", enum: values, ...(description ? { description } : {}) };
    chosen.add(name);
  }
  chosen.add(tag);
  const { oneOf: _o, anyOf: _a, ...rest } = schema;
  return {
    schema: { ...rest, type: "object", properties, required: [tag] },
    tag: {
      property: tag,
      shapes: shapes.map((shape) => ({
        allows: Object.fromEntries(
          [...chosen].filter((name) => name in shape.properties).map((name) => [name, stringChoices(shape.properties[name])]),
        ),
        required: (shape.required ?? []).filter((name) => name !== tag && !nullable(shape.properties[name])),
        nulls: (shape.required ?? []).filter((name) => name !== tag && nullable(shape.properties[name])),
        takes: Object.keys(shape.properties).filter((name) => !chosen.has(name)),
      })),
    },
  };
}

/**
 * Rules 1 to 3 everywhere in a schema, depth first: each flattened union's tag is kept under `where` in `found.tags`,
 * and each union left as it was, with why, in `found.kept`.
 */
export function simplify(doc, schema, where, found) {
  if (Array.isArray(schema)) return schema.map((each, index) => simplify(doc, each, `${where}[${index}]`, found));
  if (!isObject(schema)) return schema;
  const inner = Object.fromEntries(
    Object.entries(schema).map(([key, value]) => [key, simplify(doc, value, `${where}/${key}`, found)]),
  );
  const literal = literalUnion(inner);
  if (literal !== null) return literal;
  const nullable = nullablePair(inner);
  if (nullable !== null) return nullable;
  const tagged = taggedUnion(doc, inner);
  if (tagged === null) return inner;
  if ("conflict" in tagged) {
    found.kept[where] = tagged.conflict;
    return inner;
  }
  found.tags[where] = tagged.tag;
  return tagged.schema;
}

/** Rule 4: an answer that wraps one object, by reference, as the object. */
export function unwrapped(schema) {
  if (!isObject(schema) || schema.type !== "object" || !isObject(schema.properties)) return schema;
  const names = Object.keys(schema.properties);
  if (names.length !== 1) return schema;
  const only = schema.properties[names[0]];
  return isObject(only) && typeof only.$ref === "string" && (schema.required ?? []).includes(names[0]) ? only : schema;
}

/** What an operation takes as its body, followed through its reference. */
function bodyOf(doc, operation) {
  return resolve(doc, operation?.requestBody?.content?.["application/json"]?.schema);
}

/** What an operation answers when it succeeds, followed through its reference. */
function answerOf(doc, operation) {
  for (const [status, answer] of Object.entries(operation?.responses ?? {}))
    if (status.startsWith("2")) return resolve(doc, answer?.content?.["application/json"]?.schema);
  return undefined;
}

/** The property of an answer that holds its object: the one that names a shape of its own. */
export function objectKey(answer) {
  const named = Object.entries(answer?.properties ?? {}).filter(([, value]) => isObject(value) && typeof value.$ref === "string");
  if (named.length !== 1) throw new Error(`An answer holds ${named.length} named shapes, where one was expected`);
  return named[0][0];
}

const without = (properties, names) => Object.fromEntries(Object.entries(properties).filter(([name]) => !names.includes(name)));
const only = (properties, names) => Object.fromEntries(Object.entries(properties).filter(([name]) => names.includes(name)));

/** What a resource is made and changed with, as one shape: what making it requires, required; the rest optional. */
function settingsOf(create, change, leaves) {
  const properties = without(
    { ...create.properties, ...without(change?.properties ?? {}, Object.keys(create.properties)) },
    leaves,
  );
  return {
    type: "object",
    ...(create.description ? { description: create.description } : {}),
    properties,
    required: (create.required ?? []).filter((name) => name in properties),
  };
}

/** A monitor type's shapes in a union of them, by its type: as it's made, as it's changed, and as it's read. */
export function monitorShapes(doc, type) {
  const schemas = doc.components.schemas;
  const made = (members(schemas.CreateMonitor) ?? [])
    .map((choice) => refName(choice.$ref))
    .find((name) => schemas[name]?.properties?.type?.const === type);
  if (made === undefined) throw new Error(`No monitor is made with type "${type}"`);
  const base = made.slice("Create".length);
  return { create: schemas[made], change: schemas[`Change${base}`], read: schemas[base] };
}

/**
 * Which of its settings only making a resource takes, so a change of one makes it again, and which only a change
 * takes, sent by one straight after making it.
 */
function apart(create, change, settings) {
  const names = Object.keys(settings.properties);
  return {
    made: names.filter((name) => !(name in (change?.properties ?? {}))),
    changed: names.filter((name) => !(name in create.properties)),
  };
}

/** Where an answer holds its object, and what making it answers beside it. */
function answered(doc, resource) {
  const read = answerOf(doc, doc.paths[`${resource.path}/{id}`].get);
  const made = answerOf(doc, doc.paths[resource.path].post);
  const key = objectKey(read);
  return { key, extras: Object.keys(made.properties).filter((name) => name !== key) };
}

/**
 * Rule 5's shapes for one resource: its settings, what it's made and changed with, and what the provider keeps of it.
 * What the resource leaves is in neither; what it keeps as given is only in its settings. Beside them, which settings
 * only making or only a change takes, and where its answers hold it.
 */
export function resourceShapes(doc, resource) {
  const leaves = resource.leaves ?? [];
  if (resource.monitor !== undefined) {
    const { create, change, read } = monitorShapes(doc, resource.monitor);
    const settings = settingsOf(create, change, ["type", "expected_version", ...leaves]);
    const keeps = resource.keeps ?? [];
    return {
      ...apart(create, change, settings),
      answer: answered(doc, resource),
      settings,
      kept: {
        type: "object",
        description: `A ${resource.monitor} monitor as it was made, with its id and version: what the provider keeps of it.`,
        properties: {
          ...settings.properties,
          id: { type: "string", description: "The monitor's id." },
          version: { type: "integer", description: "Its version, which a change must name, as its last read." },
          ...only(read.properties, keeps),
        },
        required: ["id", "version", ...settings.required, ...keeps],
      },
    };
  }
  const collection = doc.paths[resource.path];
  const item = doc.paths[`${resource.path}/{id}`];
  const create = bodyOf(doc, collection.post);
  const change = bodyOf(doc, item.patch);
  const settings = settingsOf(create, change, leaves);
  const read = answerOf(doc, item.get);
  const made = answerOf(doc, collection.post);
  const key = objectKey(read);
  const object = resolve(doc, read.properties[key]);
  const properties = without(
    { ...object.properties, ...without(made.properties, [key]) },
    [...leaves, ...(resource.given ?? [])],
  );
  return {
    ...apart(create, change, settings),
    answer: answered(doc, resource),
    settings,
    kept: {
      type: "object",
      ...(object.description ? { description: object.description } : {}),
      properties,
      required: [...(object.required ?? []), ...(made.required ?? [])].filter((name) => name !== key && name in properties),
    },
  };
}

/** A resource's name as a shape's: http_monitor as HttpMonitor. */
export const pascal = (name) => name.split("_").map((word) => word[0].toUpperCase() + word.slice(1)).join("");

/** Rule 5: each resource's paths of its own, and their shapes, <Name>Settings and <Name>Kept. */
export function resourcePaths(doc, resources) {
  const paths = {};
  const schemas = {};
  for (const [name, resource] of Object.entries(resources)) {
    const { settings, kept } = resourceShapes(doc, resource);
    const shape = pascal(name);
    schemas[`${shape}Settings`] = settings;
    schemas[`${shape}Kept`] = kept;
    const named = (schema) => ({ "application/json": { schema: { $ref: `#/components/schemas/${schema}` } } });
    const body = { required: true, content: named(`${shape}Settings`) };
    const answer = { description: "What the provider keeps of it.", content: named(`${shape}Kept`) };
    paths[`/provider/${name}`] = { post: { operationId: `create_${name}`, requestBody: body, responses: { 201: answer } } };
    paths[`/provider/${name}/{id}`] = {
      parameters: [{ name: "id", in: "path", required: true, schema: { type: "string" } }],
      get: { operationId: `read_${name}`, responses: { 200: answer } },
      patch: { operationId: `change_${name}`, requestBody: body, responses: { 200: answer } },
      delete: { operationId: `delete_${name}`, responses: { 204: { description: "Deleted." } } },
    };
  }
  return { paths, schemas };
}

/** api/generator.yml: each resource read from its paths of its own. */
export function generatorConfig(resources) {
  const lines = [
    "# Written by tools/derive.mjs from tools/resources.mjs: each resource's schema, read by HashiCorp's OpenAPI generator",
    "# from api/provider.json's paths of its own (rule 5), its settings, then what the provider keeps of it.",
    "provider:",
    "  name: sustain",
    "resources:",
  ];
  for (const name of Object.keys(resources)) {
    lines.push(`  ${name}:`);
    for (const [operation, method, below] of [
      ["create", "POST", ""],
      ["read", "GET", "/{id}"],
      ["update", "PATCH", "/{id}"],
      ["delete", "DELETE", "/{id}"],
    ])
      lines.push(`    ${operation}:`, `      path: /provider/${name}${below}`, `      method: ${method}`);
  }
  return `${lines.join("\n")}\n`;
}

/**
 * The provider's copy of Sustain's description, the tags its flattened unions carry, and the unions it left as they
 * were. Simplified until nothing changes, as a union of shapes reads them as they were simplified the time before.
 */
export function derive(sustain, resources = RESOURCES) {
  const doc = structuredClone(sustain);
  const found = { tags: {}, kept: {} };
  const own = resourcePaths(doc, resources);
  doc.components.schemas = { ...doc.components.schemas, ...own.schemas };
  doc.paths = { ...doc.paths, ...own.paths };
  for (const ops of Object.values(doc.paths))
    for (const op of Object.values(ops)) {
      if (!isObject(op) || !isObject(op.responses)) continue;
      for (const answer of Object.values(op.responses)) {
        const content = answer?.content?.["application/json"];
        if (content) content.schema = unwrapped(content.schema);
      }
    }
  // A union flattened in one pass is a plain shape in the next, so its tag is kept from the pass that flattened it;
  // what's left as it was is the last pass's.
  for (let pass = 0; pass < 5; pass++) {
    const before = JSON.stringify(doc);
    found.kept = {};
    doc.components.schemas = Object.fromEntries(
      Object.entries(doc.components.schemas).map(([name, schema]) => [name, simplify(doc, schema, name, found)]),
    );
    doc.paths = simplify(doc, doc.paths, "paths", found);
    if (JSON.stringify(doc) === before) return { doc, tags: found.tags, kept: found.kept };
  }
  throw new Error("The description didn't settle in five passes");
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const sustain = JSON.parse(readFileSync(join(ROOT, "api", "sustain.json"), "utf8"));
  const { doc, tags, kept } = derive(sustain);
  writeFileSync(join(ROOT, "api", "provider.json"), `${JSON.stringify(doc, null, 2)}\n`);
  writeFileSync(join(ROOT, "api", "tags.json"), `${JSON.stringify(tags, null, 2)}\n`);
  writeFileSync(join(ROOT, "api", "generator.yml"), generatorConfig(RESOURCES));
  console.log(`api/provider.json: ${Object.keys(doc.paths).length} paths; api/tags.json: ${Object.keys(tags).length} unions flattened`);
  for (const [where, why] of Object.entries(kept)) console.log(`left as it was: ${where}: ${why}`);
}
