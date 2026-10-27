// What the provider makes of each attribute (schemas.mjs), rule by rule. Run as `node --test tools/schemas.test.mjs`.

import assert from "node:assert/strict";
import { test } from "node:test";
import { resourceOf } from "./schemas.mjs";

const named = (schema) => ({ content: { "application/json": { schema } } });
const ref = (name) => ({ $ref: `#/components/schemas/${name}` });

/** Sustain's description of one resource, `/api/v1/pages`: made with a name, a time zone and a plan; changed with more. */
const sustain = {
  components: {
    schemas: {
      Page: {
        type: "object",
        properties: { id: { type: "string" }, name: { type: "string" }, created_at: { type: "string" }, views: { type: "integer" } },
      },
    },
  },
  paths: {
    "/api/v1/pages": {
      post: {
        requestBody: named({
          type: "object",
          properties: {
            name: { type: "string" },
            plan: { type: "string" },
            depends_on: { type: "array", items: { type: "string" } },
            password: { type: "string" },
          },
        }),
        responses: { 201: named({ type: "object", properties: { page: ref("Page"), key: { type: "string" } } }) },
      },
    },
    "/api/v1/pages/{id}": {
      get: { responses: { 200: named({ type: "object", properties: { page: ref("Page") } }) } },
      patch: { requestBody: named({ type: "object", properties: { name: { type: "string" }, colour: { type: "string" } } }) },
    },
  },
};

/** provider.json's settings for it, as tools/derive.mjs would write them. */
const doc = {
  components: {
    schemas: {
      PageSettings: {
        type: "object",
        properties: {
          name: { type: "string" },
          plan: { type: "string" },
          depends_on: { type: "array", items: { type: "string" } },
          password: { type: "string" },
          colour: { type: "string", "x-nullable": true, default: null },
          target: { type: "number", "x-nullable": true, default: 99.9 },
          contract: ref("Contract"),
          check: ref("Check"),
        },
        required: ["name", "contract"],
      },
      PageKept: { type: "object", properties: { id: { type: "string" }, created_at: { type: "string" }, key: { type: "string" } } },
      Contract: {
        type: "object",
        properties: { hours: { type: "string", "x-nullable": true } },
        required: ["hours"],
      },
      Check: { type: "object", properties: { kind: { type: "string" }, words: { type: "string" } }, required: ["kind"] },
    },
  },
};
const tags = { Check: { property: "kind", shapes: [{ allows: { kind: ["words"] }, required: ["words"], nulls: [], takes: ["words"] }] } };

const attribute = (name, spec) => ({ name, ...spec });
const spec = {
  schema: {
    attributes: [
      attribute("name", { string: { computed_optional_required: "required" } }),
      attribute("plan", { string: { computed_optional_required: "computed_optional" } }),
      attribute("depends_on", { list: { computed_optional_required: "computed_optional", element_type: { string: {} } } }),
      attribute("password", { string: { computed_optional_required: "computed_optional" } }),
      // HashiCorp's generator writes a default of null as "": Sustain's description is read instead.
      attribute("colour", { string: { computed_optional_required: "computed_optional", default: { static: "" } } }),
      attribute("target", { number: { computed_optional_required: "computed_optional" } }),
      attribute("contract", {
        single_nested: {
          computed_optional_required: "required",
          attributes: [attribute("hours", { string: { computed_optional_required: "required" } })],
        },
      }),
      attribute("check", {
        single_nested: {
          computed_optional_required: "computed_optional",
          attributes: [
            attribute("kind", { string: { computed_optional_required: "required" } }),
            attribute("words", { string: { computed_optional_required: "computed_optional" } }),
          ],
        },
      }),
      attribute("id", { string: { computed_optional_required: "computed" } }),
      attribute("created_at", { string: { computed_optional_required: "computed" } }),
      attribute("key", { string: { computed_optional_required: "computed" } }),
    ],
  },
};
const rules = {
  path: "/api/v1/pages",
  writeOnly: ["password"],
  sensitive: ["key"],
  stable: ["key"],
  renamed: { depends_on: "dependency_ids" },
};

const page = () => resourceOf(spec, doc, sustain, tags, "page", rules);
const find = (attributes, name) => attributes.find((each) => each.name === name);

test("an attribute that may be null is optional, sent as null; one Sustain gives a value of its own, its default", () => {
  const { attributes, definition } = page();
  assert.deepEqual([find(attributes, "colour").mode, find(attributes, "colour").default], ["optional", undefined]);
  const target = find(attributes, "target");
  assert.equal(target.mode, "computed_optional");
  assert.equal(target.default, 'numberdefault.StaticBigFloat(decimal("99.9"))');
  assert.match(target.description, /99\.9 when it isn't given, as Terraform can't ask for null\./);
  // A contract's hours are required, but may be null: optional, and sent as null.
  assert.equal(find(find(attributes, "contract").attributes, "hours").mode, "optional");
  assert.deepEqual(definition.nulls, ["colour", "contract.hours"]);
});

test("a tagged object's fields are optional, its tag required, as its shape says the rest", () => {
  const { attributes, definition } = page();
  const check = find(attributes, "check").attributes;
  assert.equal(find(check, "kind").mode, "required");
  assert.equal(find(check, "words").mode, "optional");
  assert.deepEqual(definition.tags, [{ at: "check", tag: "Check" }]);
});

test("a write-only value is sent again when its version is raised, never kept, and never shown", () => {
  const { attributes, definition } = page();
  const password = find(attributes, "password");
  assert.deepEqual([password.mode, password.writeOnly, password.sensitive], ["optional", true, true]);
  assert.equal(find(attributes, "password_wo_version").mode, "optional");
  assert.deepEqual(definition.writeOnly, ["password"]);
});

test("what only making takes makes it again when changed; what only a change takes is sent by one after making", () => {
  const { attributes, definition } = page();
  assert.equal(find(attributes, "plan").replace, true);
  assert.equal(find(attributes, "name").replace, false);
  assert.deepEqual(definition.changed, ["colour"]);
  assert.deepEqual(definition.extras, ["key"]);
});

test("what only an answer has is computed, and kept from one plan to the next if it's an id, a making time or stable", () => {
  const { attributes } = page();
  for (const name of ["id", "created_at", "key"]) {
    assert.equal(find(attributes, name).mode, "computed", name);
    assert.equal(find(attributes, name).keep, true, name);
  }
  assert.equal(find(attributes, "key").sensitive, true);
  // Sustain's value where none is given, kept from one plan to the next.
  assert.deepEqual([find(attributes, "plan").mode, find(attributes, "plan").keep], ["computed_optional", true]);
});

test("a name Terraform keeps for its own is given another, and the provider sends Sustain's", () => {
  const { attributes, definition } = page();
  assert.ok(find(attributes, "dependency_ids"));
  assert.equal(find(attributes, "depends_on"), undefined);
  assert.deepEqual(definition.renamed, { dependency_ids: "depends_on" });
  assert.ok(definition.sends.includes("dependency_ids"));
  assert.throws(
    () => resourceOf(spec, doc, sustain, tags, "page", { ...rules, renamed: {} }),
    /"depends_on" is Terraform's own/,
  );
});
