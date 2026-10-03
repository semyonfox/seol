"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const source = fs.readFileSync(require.resolve("../assets/landing.js"), "utf8");

function fixture(options = {}) {
  const requests = [];
  const writes = [];
  const statuses = [];
  const handlers = new Map();
  const windowHandlers = new Map();
  let time = 0;
  let selected = null;
  const storage = options.storage ?? { preference: options.preference ?? null, unreadable: options.storageFails ?? false };
  let timerId = 0;
  const timers = new Map();
  const toggle = { checked: false, disabled: true, addEventListener: (_name, fn) => handlers.set("change", fn) };
  const code = {
    textContent: "seol configure --server https://private.fixture.test/p/PRIVATE-ID/?token=PRIVATE --token PRIVATE-TEXT",
    closest: () => ({ append: (status) => statuses.push(status) }),
  };
  const button = { dataset: { copy: "configure-command" }, hidden: true, addEventListener: (_name, fn) => handlers.set("click", fn) };
  const privacyStatus = { textContent: "" };
  const navigator = {
    ...options.privacy,
    clipboard: { writeText: async (value) => {
      if (options.copyFails) throw new Error("PRIVATE error https://private.fixture.test/p/PRIVATE-ID/");
      writes.push(value);
    } },
  };
  const localStorage = {
    getItem: () => { if (storage.unreadable) throw new Error("private storage error"); return storage.preference; },
    setItem: (_key, value) => { if (storage.unreadable) throw new Error("private storage error"); storage.preference = value; },
  };
  const context = {
    document: {
      getElementById: (id) => ({
        "privacy-settings": { dataset: { enabled: String(options.enabled ?? false), endpoint: options.endpoint ?? "https://collector.example.test/v1/events" } },
        "anonymous-counts": toggle, "privacy-status": privacyStatus, "configure-command": code,
      })[id],
      querySelectorAll: () => [button],
      createElement: () => ({ textContent: "", setAttribute() {} }),
      createRange: () => ({ selectNodeContents: (node) => { selected = node; } }),
    },
    location: { origin: "https://pages.example.test", pathname: "/p/PRIVATE-ID/", search: "?private=PRIVATE", href: "https://pages.example.test/p/PRIVATE-ID/?private=PRIVATE" },
    navigator,
    window: {
      doNotTrack: options.windowDNT,
      getSelection: () => ({ removeAllRanges() {}, addRange() {} }),
      addEventListener: (name, fn) => windowHandlers.set(name, fn),
    },
    localStorage, URL, AbortController,
    Date: { now: () => time },
    setTimeout: (fn, delay) => { const id = ++timerId; timers.set(id, { fn, at: time + delay }); return id; },
    clearTimeout: (id) => timers.delete(id),
    fetch: (endpoint, request) => {
      requests.push({ endpoint, request });
      if (options.transportThrows) throw new Error("PRIVATE request message");
      if (options.transportRejects) return Promise.reject(new Error("PRIVATE transport stack"));
      if (options.transportHangs) return new Promise(() => {});
      return Promise.resolve({ status: 204 });
    },
  };
  vm.runInNewContext(source, context);
  return {
    requests, writes, statuses, toggle, navigator, code, privacyStatus,
    click: () => handlers.get("click")(),
    change: (checked) => { toggle.checked = checked; handlers.get("change")(); },
    preference: () => storage.preference,
    storage,
    storageEvent: (key = "seol.telemetry.disabled") => windowHandlers.get("storage")?.({ key }),
    selected: () => selected,
    advance: (milliseconds) => {
      time += milliseconds;
      for (const [id, timer] of [...timers]) if (timer.at <= time) { timers.delete(id); timer.fn(); }
    },
  };
}

const flush = async () => { await Promise.resolve(); await Promise.resolve(); };

test("disabled configuration and every privacy opt-out emit nothing", async () => {
  const withUserInfo = new URL("https://collector.example.test/v1/events");
  withUserInfo.username = "example";
  withUserInfo.password = "example";
  for (const options of [
    {}, { enabled: true, endpoint: "" }, { enabled: true, endpoint: "http://collector.example.test/v1/events" },
    { enabled: true, endpoint: withUserInfo.href },
    { enabled: true, endpoint: "https://collector.example.test/v1/events?private=fixture" },
    { enabled: true, endpoint: "//collector.example.test/v1/events" },
    { enabled: true, preference: "true" }, { enabled: true, storageFails: true },
    { enabled: true, privacy: { globalPrivacyControl: true } },
    { enabled: true, privacy: { doNotTrack: "1" } }, { enabled: true, windowDNT: "1" },
  ]) {
    const app = fixture(options); await flush(); await app.click(); await flush();
    assert.equal(app.requests.length, 0, JSON.stringify(options));
    assert.equal(app.writes.length, 1, "privacy does not stop command copying");
  }
});

test("real view, successful copy and copy fallback send only six fixed fields", async () => {
  const app = fixture({ enabled: true }); await flush(); await app.click(); await flush();
  assert.deepEqual(app.requests.map(({ request }) => JSON.parse(request.body).name), ["screen_view", "action_completed"]);
  assert.equal(app.writes[0], app.code.textContent);
  const failed = fixture({ enabled: true, copyFails: true }); await flush(); await failed.click(); await flush();
  assert.equal(failed.selected(), failed.code);
  assert.match(failed.statuses[0].textContent, /Text selected/);
  assert.equal(JSON.parse(failed.requests[1].request.body).name, "permission_failed");
  for (const { request } of [...app.requests, ...failed.requests]) {
    const payload = JSON.parse(request.body);
    assert.deepEqual(Object.keys(payload).sort(), ["app", "kind", "name", "route", "surface", "version"]);
    assert.equal(payload.app, "seol"); assert.equal(payload.route, "home"); assert.equal(payload.surface, "web");
    assert.ok(["count", "error"].includes(payload.kind));
    assert.ok(["screen_view", "action_completed", "permission_failed"].includes(payload.name));
    assert.doesNotMatch(request.body, /PRIVATE|private\.fixture|token=|message|stack/);
    assert.equal(request.credentials, "omit"); assert.equal(request.referrerPolicy, "no-referrer");
    assert.equal(request.redirect, "error"); assert.equal(request.method, "POST");
    assert.ok(Buffer.byteLength(request.body) < 1024);
  }
});

test("opt-out persists only a boolean and takes effect before another copy", async () => {
  const app = fixture({ enabled: true }); await flush();
  app.change(false); await app.click(); await flush();
  assert.equal(app.preference(), "true"); assert.equal(app.requests.length, 1); assert.equal(app.toggle.checked, false);
  app.change(true); await app.click(); await flush();
  assert.equal(app.preference(), "false"); assert.equal(app.requests.length, 2);
  app.navigator.globalPrivacyControl = true;
  await app.click(); await flush(); assert.equal(app.requests.length, 2);
});

test("other-tab disable suppresses real copies before storage-event delivery", async () => {
  for (const copyFails of [false, true]) {
    const storage = { preference: null, unreadable: false };
    const app = fixture({ enabled: true, storage, copyFails });
    const other = fixture({ enabled: true, storage });
    await flush();
    other.change(false);
    await app.click(); await flush();
    assert.equal(app.requests.length, 1, "no count or error after another tab disables");
    assert.equal(app.toggle.checked, false);
    assert.match(app.privacyStatus.textContent, /counts are off/);
    if (copyFails) {
      assert.equal(app.selected(), app.code);
      assert.match(app.statuses[0].textContent, /Text selected/);
    } else {
      assert.equal(app.writes[0], app.code.textContent);
      assert.match(app.statuses[0].textContent, /^Copied/);
    }
  }
});

test("storage becoming unreadable suppresses successful and fallback copy telemetry", async () => {
  for (const copyFails of [false, true]) {
    const app = fixture({ enabled: true, copyFails }); await flush();
    app.storage.unreadable = true;
    await app.click(); await flush();
    assert.equal(app.requests.length, 1, "unreadable preference fails closed after load");
    assert.equal(app.toggle.checked, false);
    assert.match(app.privacyStatus.textContent, /counts are off/);
    if (copyFails) {
      assert.equal(app.selected(), app.code);
      assert.match(app.statuses[0].textContent, /Text selected/);
    } else {
      assert.equal(app.writes[0], app.code.textContent);
      assert.match(app.statuses[0].textContent, /^Copied/);
    }
  }
});

test("preference events refresh controls and cancel pending transport without retries", async () => {
  for (const unreadable of [false, true]) {
    const app = fixture({ enabled: true, transportHangs: true });
    app.storage.preference = "true";
    app.storage.unreadable = unreadable;
    app.storageEvent("unrelated-preference");
    assert.equal(app.requests[0].request.signal.aborted, false);
    app.storageEvent();
    assert.equal(app.requests[0].request.signal.aborted, true);
    assert.equal(app.toggle.checked, false);
    assert.match(app.privacyStatus.textContent, /counts are off/);
    app.advance(60_000); await app.click(); await flush();
    assert.equal(app.requests.length, 1, "cancellation does not retry");
    assert.equal(app.writes.length, 1);
  }
  const cleared = fixture({ enabled: true, preference: "true" });
  cleared.storage.preference = null;
  cleared.storageEvent(null);
  assert.equal(cleared.toggle.checked, true);
  assert.match(cleared.privacyStatus.textContent, /counts are on/);
  assert.equal(cleared.requests.length, 0, "storage changes never send automatically");
  await cleared.click(); await flush();
  assert.equal(cleared.requests.length, 1);
  cleared.storage.unreadable = true;
  cleared.storageEvent(null);
  assert.equal(cleared.toggle.checked, false, "storage clear also fails closed when unreadable");
});

test("other-tab enable or storage clear cannot override this tab's explicit disable", async () => {
  const app = fixture({ enabled: true }); await flush();
  app.change(false);
  for (const preference of ["false", null]) {
    app.storage.preference = preference;
    app.storageEvent(preference === null ? null : "seol.telemetry.disabled");
    assert.equal(app.toggle.checked, false);
    await app.click(); await flush();
    assert.equal(app.requests.length, 1);
  }
  assert.equal(app.writes.length, 2);
});

test("transport failures cannot change copying or create retries", async () => {
  for (const options of [{ transportThrows: true }, { transportRejects: true }]) {
    const app = fixture({ enabled: true, ...options }); await flush(); await app.click(); await flush();
    assert.equal(app.writes.length, 1); assert.match(app.statuses[0].textContent, /^Copied/);
    assert.equal(app.requests.length, 2); app.advance(60_000); await flush(); assert.equal(app.requests.length, 2);
  }
});

test("rate, lifetime, repeated-error and in-flight limits bound requests", async () => {
  const app = fixture({ enabled: true }); await flush();
  for (let i = 0; i < 30; i++) { await app.click(); await flush(); }
  assert.equal(app.requests.length, 20);
  for (let minute = 0; minute < 15; minute++) {
    app.advance(60_000);
    for (let i = 0; i < 20; i++) { await app.click(); await flush(); }
  }
  assert.equal(app.requests.length, 200);
  const errors = fixture({ enabled: true, copyFails: true }); await flush();
  for (let i = 0; i < 5; i++) { await errors.click(); await flush(); }
  assert.equal(errors.requests.length, 2); errors.advance(60_000); await errors.click(); await flush(); assert.equal(errors.requests.length, 3);
  const hanging = fixture({ enabled: true, transportHangs: true });
  await hanging.click(); assert.equal(hanging.requests.length, 1);
  hanging.advance(2_000); assert.equal(hanging.requests[0].request.signal.aborted, true);
  assert.equal(hanging.requests.length, 1, "timeout does not retry");
  await hanging.click(); assert.equal(hanging.requests.length, 2);
  hanging.change(false); assert.equal(hanging.requests[1].request.signal.aborted, true);
});
