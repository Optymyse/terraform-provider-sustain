// The provider's copy of Sustain's description (derive.mjs), rule by rule. Run as `node --test tools/derive.test.mjs`.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import {
  derive,
  generatorConfig,
  literalUnion,
  nullablePair,
  resourcePaths,
  resourceShapes,
  taggedUnion,
  unwrapped,
} from "./derive.mjs";
import { RESOURCES } from "./resources.mjs";

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..");

test("a union of one type's literals is that type's list of values", () => {
  assert.deepEqual(
    literalUnion({ anyOf: [{ type: "number", const: 1 }, { type: "number", const: 7 }], description: "Days." }),
    { type: "number", enum: [1, 7], description: "Days." },
  );
  // Literals of two types, or a union with anything else in it, aren't one.
  assert.equal(literalUnion({ anyOf: [{ type: "number", const: 1 }, { type: "string", const: "a" }] }), null);
  assert.equal(literalUnion({ anyOf: [{ type: "number", const: 1 }, { type: "number" }] }), null);
});

test("a value or null is the value, marked as one that may be null, keeping the pair's words where it has none", () => {
  assert.deepEqual(nullablePair({ anyOf: [{ type: "string" }, { type: "null" }], description: "Or none." }), {
    type: "string",
    description: "Or none.",
    "x-nullable": true,
  });
  assert.deepEqual(nullablePair({ anyOf: [{ $ref: "#/components/schemas/Area" }, { type: "null" }] }), {
    $ref: "#/components/schemas/Area",
    "x-nullable": true,
  });
  assert.deepEqual(nullablePair({ anyOf: [{ type: "number", enum: [7, 14] }, { type: "null" }], default: 14 }), {
    type: "number",
    enum: [7, 14],
    default: 14,
    "x-nullable": true,
  });
  assert.equal(nullablePair({ anyOf: [{ type: "string" }, { type: "number" }] }), null);
});

test("shapes chosen by a tag are one shape, each kept as what it allows, requires and takes", () => {
  const doc = { components: { schemas: {} } };
  const status = {
    oneOf: [
      {
        type: "object",
        properties: { mode: { type: "string", const: "class" }, status_class: { type: "number", enum: [1, 2] } },
        required: ["mode", "status_class"],
      },
      {
        type: "object",
        properties: {
          mode: { type: "string", const: "range" },
          min_status_code: { type: "integer" },
          max_status_code: { type: "integer" },
        },
        required: ["mode", "min_status_code", "max_status_code"],
      },
    ],
    description: "A class, or a range.",
  };
  const { schema, tag } = taggedUnion(doc, status);
  assert.deepEqual(schema, {
    description: "A class, or a range.",
    type: "object",
    properties: {
      mode: { type: "string", enum: ["class", "range"] },
      status_class: { type: "number", enum: [1, 2] },
      min_status_code: { type: "integer" },
      max_status_code: { type: "integer" },
    },
    required: ["mode"],
  });
  assert.deepEqual(tag, {
    property: "mode",
    shapes: [
      { allows: { mode: ["class"] }, required: ["status_class"], nulls: [], takes: ["status_class"] },
      {
        allows: { mode: ["range"] },
        required: ["min_status_code", "max_status_code"],
        nulls: [],
        takes: ["min_status_code", "max_status_code"],
      },
    ],
  });
});

test("shapes told apart by two strings keep each one's choices of both", () => {
  const doc = { components: { schemas: {} } };
  const assertion = {
    anyOf: [
      {
        type: "object",
        properties: { type: { type: "string", const: "json_path" }, condition: { type: "string", enum: ["exists", "not_exists"] }, path: { type: "string" } },
        required: ["type", "condition", "path"],
      },
      {
        type: "object",
        properties: { type: { type: "string", const: "json_path" }, condition: { type: "string", const: "equals" }, path: { type: "string" }, expected: { type: "string" } },
        required: ["type", "condition", "path", "expected"],
      },
    ],
  };
  const { schema, tag } = taggedUnion(doc, assertion);
  assert.deepEqual(schema.properties.condition, { type: "string", enum: ["exists", "not_exists", "equals"] });
  assert.deepEqual(
    tag.shapes.map((shape) => [shape.allows, shape.required]),
    [
      [{ type: ["json_path"], condition: ["exists", "not_exists"] }, ["condition", "path"]],
      [{ type: ["json_path"], condition: ["equals"] }, ["condition", "path", "expected"]],
    ],
  );
});

test("a property one shape requires but lets be null is one thing in every shape, sent as null when it isn't given", () => {
  const doc = { components: { schemas: {} } };
  const target = { $ref: "#/components/schemas/BrowserTarget" };
  const step = {
    anyOf: [
      {
        type: "object",
        properties: { action: { type: "string", const: "click" }, target },
        required: ["action", "target"],
      },
      {
        type: "object",
        properties: { action: { type: "string", const: "expect" }, target: { ...target, "x-nullable": true } },
        required: ["action", "target"],
      },
    ],
  };
  const { schema, tag } = taggedUnion(doc, step);
  assert.deepEqual(schema.properties.target, { ...target, "x-nullable": true });
  assert.deepEqual(
    tag.shapes.map((shape) => [shape.required, shape.nulls]),
    [
      [["target"], []],
      [[], ["target"]],
    ],
  );
});

test("shapes that make one property two different things are left as they are, with why", () => {
  const doc = { components: { schemas: {} } };
  const monitor = {
    oneOf: [
      { type: "object", properties: { type: { type: "string", const: "http" }, config: { type: "object", properties: { url: { type: "string" } } } } },
      { type: "object", properties: { type: { type: "string", const: "tcp" }, config: { type: "object", properties: { port: { type: "integer" } } } } },
    ],
  };
  assert.deepEqual(taggedUnion(doc, monitor), { conflict: '"config" is a different thing in two of its shapes' });
});

test("an answer wrapping its object is the object, and any other answer is left as it is", () => {
  const service = { type: "object", properties: { service: { $ref: "#/components/schemas/Service" } }, required: ["service"] };
  assert.deepEqual(unwrapped(service), { $ref: "#/components/schemas/Service" });
  const page = { type: "object", properties: { monitors: { type: "array" }, next: { type: "string" } } };
  assert.equal(unwrapped(page), page);
});

test("a monitor is kept as it was made, with its id, its version and what it keeps of its answer", () => {
  const monitor = { content: { "application/json": { schema: { type: "object", properties: { monitor: { $ref: "#/components/schemas/HeartbeatMonitor" } } } } } };
  const doc = {
    paths: {
      "/api/v1/monitors": { post: { responses: { 201: monitor } } },
      "/api/v1/monitors/{id}": { get: { responses: { 200: monitor } } },
    },
    components: {
      schemas: {
        CreateMonitor: { oneOf: [{ $ref: "#/components/schemas/CreateHeartbeatMonitor" }] },
        CreateHeartbeatMonitor: {
          type: "object",
          properties: { type: { type: "string", const: "heartbeat" }, name: { type: "string" }, period_seconds: { type: "integer" } },
          required: ["type", "name", "period_seconds"],
        },
        ChangeHeartbeatMonitor: {
          type: "object",
          properties: { expected_version: { type: "integer" }, name: { type: "string" }, paused_note: { type: "string" } },
          required: ["expected_version"],
        },
        HeartbeatMonitor: {
          type: "object",
          properties: { id: { type: "string" }, name: { type: "string" }, address: { type: "string" }, status: { type: "string" } },
        },
      },
    },
  };
  const { settings, kept, made, changed, answer } = resourceShapes(doc, {
    path: "/api/v1/monitors",
    monitor: "heartbeat",
    keeps: ["address"],
  });
  // Made with its own fields and changed with any of them, or with what only a change takes; never its type or version.
  assert.deepEqual(Object.keys(settings.properties), ["name", "period_seconds", "paused_note"]);
  assert.deepEqual(settings.required, ["name", "period_seconds"]);
  assert.deepEqual([made, changed], [["period_seconds"], ["paused_note"]]);
  assert.deepEqual(answer, { key: "monitor", extras: [] });
  assert.deepEqual(Object.keys(kept.properties), ["name", "period_seconds", "paused_note", "id", "version", "address"]);
  assert.deepEqual(kept.required, ["id", "version", "name", "period_seconds", "address"]);
});

test("anything else is kept as it's read, with what its making answered, but what it leaves or keeps as given", () => {
  const named = (schema) => ({ content: { "application/json": { schema } } });
  const doc = {
    components: {
      schemas: {
        Channel: {
          type: "object",
          properties: { id: { type: "string" }, name: { type: "string" }, failures: { type: "integer" }, url: { type: "string" } },
          required: ["id", "name", "failures", "url"],
        },
      },
    },
    paths: {
      "/api/v1/channels": {
        post: {
          requestBody: named({ type: "object", properties: { name: { type: "string" }, url: { type: "string" } }, required: ["name"] }),
          responses: {
            201: named({
              type: "object",
              properties: { channel: { $ref: "#/components/schemas/Channel" }, secret: { type: "string" } },
              required: ["channel", "secret"],
            }),
          },
        },
      },
      "/api/v1/channels/{id}": {
        get: { responses: { 200: named({ type: "object", properties: { channel: { $ref: "#/components/schemas/Channel" } } }) } },
        patch: { requestBody: named({ type: "object", properties: { name: { type: "string" }, muted: { type: "boolean" } } }) },
      },
    },
  };
  const resource = { path: "/api/v1/channels", leaves: ["failures"], given: ["url"] };
  const { settings, kept, made, changed, answer } = resourceShapes(doc, resource);
  assert.deepEqual(Object.keys(settings.properties), ["name", "url", "muted"]);
  assert.deepEqual(settings.required, ["name"]);
  // Its address only making takes, so a change of it makes it again; muting only a change takes.
  assert.deepEqual([made, changed], [["url"], ["muted"]]);
  assert.deepEqual(answer, { key: "channel", extras: ["secret"] });
  assert.deepEqual(Object.keys(kept.properties), ["id", "name", "secret"]);
  assert.deepEqual(kept.required, ["id", "name", "secret"]);
  const { paths } = resourcePaths(doc, { channel: resource });
  assert.deepEqual(Object.keys(paths), ["/provider/channel", "/provider/channel/{id}"]);
  assert.equal(paths["/provider/channel/{id}"].get.responses[200].content["application/json"].schema.$ref, "#/components/schemas/ChannelKept");
});

test("the generator reads each resource from its paths of its own", () => {
  const config = generatorConfig({ secret: {} });
  assert.match(config, /^resources:\n  secret:\n    create:\n      path: \/provider\/secret\n      method: POST\n/m);
  assert.match(config, /    delete:\n      path: \/provider\/secret\/\{id\}\n      method: DELETE\n$/);
});

test("Sustain's description, as kept here, makes the copy kept here", () => {
  const sustain = JSON.parse(readFileSync(join(ROOT, "api", "sustain.json"), "utf8"));
  const { doc, tags } = derive(sustain);
  assert.deepEqual(doc, JSON.parse(readFileSync(join(ROOT, "api", "provider.json"), "utf8")));
  assert.deepEqual(tags, JSON.parse(readFileSync(join(ROOT, "api", "tags.json"), "utf8")));
  assert.equal(generatorConfig(RESOURCES), readFileSync(join(ROOT, "api", "generator.yml"), "utf8"));
});
