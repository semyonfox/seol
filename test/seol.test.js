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

test("get resolves a successful HTTPS download", async () => {
  const client = {
    get(_url, _options, callback) {
      const request = new EventEmitter();
      request.setTimeout = () => {};
      request.destroy = () => {};
      const stream = response(200);
      process.nextTick(() => {
        callback(stream);
        stream.emit("data", Buffer.from("release"));
        stream.emit("end");
      });
      return request;
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
      const request = new EventEmitter();
      request.setTimeout = () => {};
      request.destroy = () => {};
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
      return request;
    },
  };

  assert.deepEqual(await get("https://example.test/original", client), Buffer.from("redirected release"));
  assert.deepEqual(urls, ["https://example.test/original", "https://example.test/redirected"]);
  assert.equal(resumed, true);
});

test("get rejects HTTPS request errors", async () => {
  const request = new EventEmitter();
  request.setTimeout = () => {};
  request.destroy = () => {};
  const client = {
    get() {
      process.nextTick(() => request.emit("error", new Error("connection failed")));
      return request;
    },
  };

  await assert.rejects(get("https://example.test/error", client), new Error("connection failed"));
});

test("get destroys an HTTPS request after 30 seconds", async () => {
  let timeout;
  const request = new EventEmitter();
  request.setTimeout = (milliseconds, callback) => {
    timeout = { milliseconds, callback };
  };
  request.destroy = (error) => process.nextTick(() => request.emit("error", error));
  const client = {
    get() {
      return request;
    },
  };

  const download = get("https://example.test/slow", client);
  assert.equal(timeout.milliseconds, 30_000);
  timeout.callback();
  await assert.rejects(download, new Error("download timed out"));
});
