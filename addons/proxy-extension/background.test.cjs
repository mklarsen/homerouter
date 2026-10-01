const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

function createBackground() {
  const handlers = {};
  const state = {};
  const fetchRequests = [];
  let proxySetCount = 0;
  let proxyClearCount = 0;

  function event(name) {
    return {
      addListener(listener, filter, extraInfoSpec) {
        handlers[name] = { listener, filter, extraInfoSpec };
      }
    };
  }

  const chrome = {
    storage: {
      local: {
        get(_keys, callback) {
          callback({ ...state });
        },
        set(values, callback) {
          Object.assign(state, values);
          if (callback) callback();
        }
      },
      onChanged: event("storageChanged")
    },
    proxy: {
      settings: {
        set(_settings, callback) {
          proxySetCount += 1;
          callback();
        },
        clear(_settings, callback) {
          proxyClearCount += 1;
          callback();
        },
        get(_details, callback) {
          callback({ levelOfControl: "controlled_by_this_extension" });
        }
      }
    },
    runtime: {
      lastError: null,
      onMessage: event("message"),
      getURL: value => value
    },
    webRequest: {
      onAuthRequired: event("auth"),
      onCompleted: event("completed"),
      onErrorOccurred: event("error")
    },
    tabs: { update() {} }
  };

  const context = vm.createContext({
    chrome,
    console,
    AbortController,
    Map,
    URL,
    setTimeout,
    clearTimeout,
    fetch: async (url, options = {}) => {
      fetchRequests.push({ url, options });
      return { ok: true, status: 200, json: async () => ({ closed_connections: 2 }) };
    }
  });
  const source = fs.readFileSync(path.join(__dirname, "background.js"), "utf8");
  vm.runInContext(source, context, { filename: "background.js" });

  return {
    handlers,
    context,
    sendMessage(message) {
      let response;
      handlers.message.listener(message, {}, value => { response = value; });
      return response;
    },
    sendMessageAsync(message) {
      return new Promise(resolve => handlers.message.listener(message, {}, resolve));
    },
    fetchRequests,
    state,
    proxySetCount: () => proxySetCount,
    proxyClearCount: () => proxyClearCount
  };
}

function plain(value) {
  return JSON.parse(JSON.stringify(value));
}

test("missing credentials cancel proxy auth and refuse connect", () => {
  const background = createBackground();
  const response = background.sendMessage({
    action: "connect",
    payload: { host: "vpn.homerouter.io", port: 443, username: "", password: "" }
  });
  assert.equal(response.status, "error");
  assert.equal(background.proxySetCount(), 0);
  assert.ok(background.proxyClearCount() > 0, "stale extension-owned proxy setting should be cleared");

  let authResponse;
  background.handlers.auth.listener(
    { isProxy: true, requestId: "no-credentials" },
    value => { authResponse = value; }
  );
  assert.deepEqual(plain(authResponse), { cancel: true });
});

test("MV3 proxy auth uses asyncBlocking and cancels repeated challenges", () => {
  const background = createBackground();
  const validConfig = {
    host: "vpn.homerouter.io",
    port: 443,
    username: "alice",
    password: "a-long-proxy-password-123"
  };
  assert.equal(background.sendMessage({ action: "connect", payload: validConfig }).status, "connected");
  assert.deepEqual(plain(background.handlers.auth.extraInfoSpec), ["asyncBlocking"]);

  let firstResponse;
  background.handlers.auth.listener(
    { isProxy: true, requestId: "repeated-request" },
    value => { firstResponse = value; }
  );
  assert.deepEqual(plain(firstResponse), {
    authCredentials: { username: "alice", password: "a-long-proxy-password-123" }
  });

  let retryResponse;
  background.handlers.auth.listener(
    { isProxy: true, requestId: "repeated-request" },
    value => { retryResponse = value; }
  );
  assert.deepEqual(plain(retryResponse), { cancel: true });
});

test("connection test refuses missing proxy credentials before setting proxy", () => {
  const background = createBackground();
  const response = background.sendMessage({
    action: "test-connection",
    payload: { host: "vpn.homerouter.io", port: 443, username: "alice", password: "" }
  });
  assert.equal(response.success, false);
  assert.match(response.error, /brugernavn.*adgangskode/i);
  assert.equal(background.proxySetCount(), 0);
});

test("temporary test credentials are limited to the test request", () => {
  const background = createBackground();
  const selectCredentials = vm.runInContext("credentialsForRequest", background.context);
  const temporary = { username: "new-user", password: "temporary-password-123" };
  const saved = { username: "saved-user", password: "saved-password-456" };
  const testURL = "https://1.1.1.1/cdn-cgi/trace";

  assert.deepEqual(
    plain(selectCredentials({ url: testURL }, temporary, testURL, saved)),
    temporary
  );
  assert.deepEqual(
    plain(selectCredentials({ url: "https://example.com/" }, temporary, testURL, saved)),
    saved
  );
});

test("explicit disconnect sends credentials over HTTPS and returns server close count", async () => {
  const background = createBackground();
  const previousClearCount = background.proxyClearCount();
  const response = await background.sendMessageAsync({
    action: "disconnect",
    payload: { username: "alice", password: "a-long-proxy-password-123" }
  });

  assert.equal(background.proxyClearCount(), previousClearCount + 1);
  assert.equal(background.state.connected, false);
  assert.equal(response.status, "disconnected");
  assert.equal(response.remoteCloseConfirmed, true);
  assert.equal(response.closedConnections, 2);
  assert.equal(background.fetchRequests.length, 1);
  assert.equal(background.fetchRequests[0].url, "https://vpn.homerouter.io/api/v1/proxy/disconnect");
  assert.equal(background.fetchRequests[0].options.method, "POST");
  assert.deepEqual(JSON.parse(background.fetchRequests[0].options.body), {
    username: "alice",
    password: "a-long-proxy-password-123"
  });
});
