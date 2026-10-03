"use strict";

(() => {
  const settings = document.getElementById("privacy-settings");
  const toggle = document.getElementById("anonymous-counts");
  const privacyStatus = document.getElementById("privacy-status");
  const endpoint = settings?.dataset.endpoint ?? "";
  let validEndpoint = false;
  try {
    const url = endpoint.startsWith("/") ? new URL(endpoint, location.origin) : new URL(endpoint);
    validEndpoint = endpoint !== "" && !/[\s\\?#]/u.test(endpoint) &&
      !endpoint.startsWith("//") && !url.username && !url.password &&
      (url.protocol === "https:" || (endpoint.startsWith("/") && url.origin === location.origin));
  } catch {
    // missing or invalid configuration keeps counts off
  }
  const configured = settings?.dataset.enabled === "true" && validEndpoint;
  let optedOut = false;
  if (configured) {
    try {
      const preference = localStorage.getItem("seol.telemetry.disabled");
      optedOut = preference !== null && preference !== "false";
    } catch {
      optedOut = true;
    }
  }
  let total = 0;
  let times = [];
  let lastTime = 0;
  let lastError = -Infinity;
  let pending = null;

  function privacySignal() {
    try {
      return navigator.globalPrivacyControl === true ||
        [navigator.doNotTrack, navigator.msDoNotTrack, window.doNotTrack].some(
          (value) => value === "1" || value === "yes" || value === 1 || value === true,
        );
    } catch {
      return true;
    }
  }

  function updatePrivacy() {
    if (!toggle || !privacyStatus) return;
    toggle.disabled = !configured || privacySignal();
    toggle.checked = !toggle.disabled && !optedOut;
    privacyStatus.textContent = toggle.checked ? "Anonymous counts are on." :
      privacySignal() ? "Anonymous counts are off because your browser requests privacy." :
      "Anonymous counts are off.";
  }

  // only fixed categories leave this page; command text and errors stay local
  function emit(kind, name) {
    try {
      if (!configured || optedOut || privacySignal() || pending || total >= 200) return;
      if (!((kind === "count" && ["screen_view", "action_completed"].includes(name)) ||
        (kind === "error" && name === "permission_failed"))) return;
      const now = Math.max(Date.now(), lastTime);
      lastTime = now;
      times = times.filter((time) => now - time < 60_000);
      if (times.length >= 20 || (kind === "error" && now - lastError < 60_000)) return;
      if (typeof fetch !== "function" || typeof AbortController !== "function") return;
      const controller = new AbortController();
      pending = controller;
      total += 1;
      times.push(now);
      if (kind === "error") lastError = now;
      const finish = () => {
        clearTimeout(timeout);
        if (pending === controller) pending = null;
      };
      const timeout = setTimeout(() => { controller.abort(); finish(); }, 2_000);
      try {
        Promise.resolve(fetch(endpoint, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ version: 1, app: "seol", kind, name, surface: "web", route: "home" }),
          credentials: "omit",
          referrerPolicy: "no-referrer",
          redirect: "error",
          cache: "no-store",
          signal: controller.signal,
        })).then(finish, finish);
      } catch {
        finish();
      }
    } catch {
      // optional statistics cannot interrupt copying or navigation
    }
  }

  if (toggle) {
    toggle.addEventListener("change", () => {
      optedOut = !toggle.checked;
      try {
        localStorage.setItem("seol.telemetry.disabled", String(optedOut));
      } catch {
        optedOut = true;
      }
      if (optedOut) pending?.abort();
      updatePrivacy();
    });
  }
  updatePrivacy();
  emit("count", "screen_view");

  for (const button of document.querySelectorAll("[data-copy]")) {
    const code = document.getElementById(button.dataset.copy);
    if (!code) continue;
    const status = document.createElement("p");
    status.className = "status";
    status.setAttribute("role", "status");
    status.setAttribute("aria-live", "polite");
    status.setAttribute("aria-atomic", "true");
    code.closest(".snippet").append(status);
    button.hidden = false;
    button.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(code.textContent);
        status.textContent = "Copied. Paste into your terminal or conversation.";
        emit("count", "action_completed");
      } catch {
        const selection = window.getSelection();
        if (selection) {
          const range = document.createRange();
          range.selectNodeContents(code);
          selection.removeAllRanges();
          selection.addRange(range);
          status.textContent = "Automatic copy is unavailable. Text selected. Use your browser's Copy command.";
        } else {
          status.textContent = "Automatic copy is unavailable. Select the text and use your browser's Copy command.";
        }
        emit("error", "permission_failed");
      }
    });
  }
})();
