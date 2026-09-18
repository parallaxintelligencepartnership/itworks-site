// embed.test.mjs is a behavioural test for embed.js against a minimal DOM
// stub. Run with: node --test web/static/
import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const source = fs.readFileSync(path.join(here, "embed.js"), "utf8");

// makeElement is the stub node embed.js is allowed to touch: textContent,
// setAttribute/getAttribute, appendChild, children, className. Nothing
// else of the real DOM exists here.
function makeElement(tag) {
  return {
    tagName: tag,
    id: "",
    className: "",
    textContent: "",
    children: [],
    attrs: {},
    setAttribute(name, value) {
      this.attrs[name] = String(value);
    },
    getAttribute(name) {
      return Object.prototype.hasOwnProperty.call(this.attrs, name)
        ? this.attrs[name]
        : null;
    },
    appendChild(child) {
      this.children.push(child);
      return child;
    },
  };
}

// collectText mimics what a browser's textContent would report for a
// subtree: this stub does not compute it live, so the test walks the tree
// itself to check what a reader would see.
function collectText(node) {
  var s = node.textContent || "";
  for (var i = 0; i < node.children.length; i++) {
    s += collectText(node.children[i]);
  }
  return s;
}

// run loads embed.js fresh, with a stub document rooted at one container
// element, and the given fetch stub. It returns the container once the
// fetch chain (a handful of promise ticks) has settled.
async function run(fetchStub) {
  const container = makeElement("div");
  const doc = {
    readyState: "complete",
    createElement: makeElement,
    getElementById: function () {
      return null;
    },
    head: { appendChild: function () {} },
    querySelectorAll: function () {
      return [container];
    },
    addEventListener: function () {},
  };
  const context = {
    document: doc,
    fetch: fetchStub,
    console: console,
  };
  vm.createContext(context);
  vm.runInContext(source, context);

  // The fetch chain in embed.js is fetch().then().then().catch(); give it
  // a few microtask/macrotask turns to resolve.
  for (var i = 0; i < 5; i++) {
    await new Promise(function (resolve) {
      setTimeout(resolve, 0);
    });
  }
  return container;
}

test("fetch rejects: shows the empty state", async () => {
  const container = await run(function () {
    return Promise.reject(new Error("network down"));
  });
  assert.ok(collectText(container).includes("No entries yet."));
});

test("empty entries: shows the empty state", async () => {
  const container = await run(function () {
    return Promise.resolve({
      ok: true,
      json: function () {
        return Promise.resolve({ entries: [] });
      },
    });
  });
  assert.ok(collectText(container).includes("No entries yet."));
});

test("an entry with an invalid id renders nothing for it", async () => {
  const container = await run(function () {
    return Promise.resolve({
      ok: true,
      json: function () {
        return Promise.resolve({
          entries: [
            {
              id: "../../evil",
              name: "<b>x</b>",
              summary: "s",
              audit_tier: "audit",
              audit_date: "2026-01-01",
              badge_color: "green",
              critical_open: 0,
              critical_accepted: 0,
            },
          ],
        });
      },
    });
  });
  const list = container.children.find(function (c) {
    return c.tagName === "ol";
  });
  assert.ok(list, "the wall list was rendered");
  assert.equal(list.children.length, 0);
});

test("a valid entry renders safely from its id alone", async () => {
  const container = await run(function () {
    return Promise.resolve({
      ok: true,
      json: function () {
        return Promise.resolve({
          entries: [
            {
              id: "abc234defg56",
              name: "<b>x</b>",
              summary: "s",
              audit_tier: "audit",
              audit_date: "2026-01-01",
              badge_color: "red",
              critical_open: 1,
              critical_accepted: 0,
              entry_url: "https://evil.example/",
              badge_url: "https://evil.example/badge.svg",
            },
          ],
        });
      },
    });
  });

  const list = container.children.find(function (c) {
    return c.tagName === "ol";
  });
  assert.ok(list, "the wall list was rendered");
  assert.equal(list.children.length, 1);

  const item = list.children[0];
  const badgeLink = item.children[0];
  const badgeImg = badgeLink.children[0];
  const body = item.children[1];
  const nameLink = body.children[0];

  assert.equal(nameLink.textContent, "<b>x</b>");
  assert.ok(nameLink.attrs.href.startsWith("https://itworks.build/e/abc234defg56/"));
  assert.ok(badgeLink.attrs.href.startsWith("https://itworks.build/e/abc234defg56/"));
  assert.equal(badgeImg.attrs.src, "https://itworks.build/badge/abc234defg56.svg");

  const text = collectText(item);
  assert.ok(text.includes("critical open"));
  assert.ok(text.includes("1 critical open"));
});
