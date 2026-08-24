"use strict";

const { EventEmitter } = require("node:events");
const test = require("node:test");
const assert = require("node:assert/strict");
const { get } = require("../scripts/seol.js");

function response(statusCode, headers = {}) {
  const stream = new EventEmitter();
  stream.statusCode = statusCode;
  stream.headers = headers;
  stream.resume = () => {};
  return stream;
}

function request() {
  const result = new EventEmitter();
  result.setTimeout = () => {};
  result.destroy = (error) => process.nextTick(() => result.emit("error", error));
  return result;
}

test("get resolves a successful HTTPS download", async () => {
  const client = {
    get(_url, _options, callback) {
      const stream = response(200);
      process.nextTick(() => {
        callback(stream);
        stream.emit("data", Buffer.from("release"));
        stream.emit("end");
      });
      return request();
    },
  };

  assert.deepEqual(await get("https://example.test/release", client), Buffer.from("release"));
});

test("get follows HTTPS redirects", async () => {
  const urls = [];
  const firstResponse = response(302, { location: "/redirected" });
  let resumed = false;
  firstResponse.resume = () => { resumed = true; };
  const client = {
    get(url, _options, callback) {
      urls.push(url);
      const result = request();
      process.nextTick(() => {
        if (urls.length === 1) {
          callback(firstResponse);
          return;
        }
        const stream = response(200);
        callback(stream);
        stream.emit("data", Buffer.from("redirected release"));
        stream.emit("end");
      });
      return result;
    },
  };

  assert.deepEqual(await get("https://example.test/original", client), Buffer.from("redirected release"));
  assert.deepEqual(urls, ["https://example.test/original", "https://example.test/redirected"]);
  assert.equal(resumed, true);
});

test("get rejects redirect loops", async () => {
  const client = {
    get(_url, _options, callback) {
      const stream = response(302, { location: "/again" });
      process.nextTick(() => callback(stream));
      return request();
    },
  };

  await assert.rejects(get("https://example.test/original", client), new Error("download exceeded redirect limit"));
});

test("get rejects downloads larger than 64 MiB", async () => {
  const client = {
    get(_url, _options, callback) {
      const stream = response(200);
      process.nextTick(() => {
        callback(stream);
        stream.emit("data", Buffer.alloc(64 * 1024 * 1024 + 1));
      });
      return request();
    },
  };

  await assert.rejects(get("https://example.test/release", client), new Error("download exceeds 64 MiB limit"));
});

test("get rejects HTTPS request errors", async () => {
  const result = request();
  const client = {
    get() {
      process.nextTick(() => result.emit("error", new Error("connection failed")));
      return result;
    },
  };

  await assert.rejects(get("https://example.test/error", client), new Error("connection failed"));
});

test("get destroys an HTTPS request after 30 seconds", async () => {
  let timeout;
  const result = request();
  result.setTimeout = (milliseconds, callback) => {
    timeout = { milliseconds, callback };
  };
  result.destroy = (error) => process.nextTick(() => result.emit("error", error));
  const client = { get: () => result };

  const download = get("https://example.test/slow", client);
  assert.equal(timeout.milliseconds, 30_000);
  timeout.callback();
  await assert.rejects(download, new Error("download timed out"));
});
