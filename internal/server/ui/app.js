(function () {
    "use strict";

    var STATUS_CLASSES = { healthy: "healthy", degraded: "degraded", unhealthy: "unhealthy" };
    var SUBSYSTEM_OK = ["connected", "running"];
    var SUBSYSTEM_WARN = ["degraded"];
    var logBuffer = [];
    var logFilter = "all";

    function $(id) {
        return document.getElementById(id);
    }

    function setText(id, value) {
        var el = $(id);
        if (el) {
            el.textContent = value;
        }
    }

    function fetchJSON(url) {
        return fetch(url).then(function (res) {
            if (!res.ok) {
                throw new Error("HTTP " + res.status);
            }
            return res.json();
        });
    }

    function renderOverview(data) {
        var status = data.status || "unknown";
        setText("stat-status", status);

        var badge = $("status-badge");
        if (badge) {
            badge.textContent = status;
            badge.className = "badge " + (STATUS_CLASSES[status] || "");
        }

        setText("stat-uptime", data.uptime_seconds != null ? formatUptime(data.uptime_seconds) : "--");
        setText("stat-version", data.version || "--");
    }

    function formatUptime(seconds) {
        if (seconds < 60) {
            return seconds + "s";
        }
        if (seconds < 3600) {
            return Math.floor(seconds / 60) + "m " + (seconds % 60) + "s";
        }
        var h = Math.floor(seconds / 3600);
        var m = Math.floor((seconds % 3600) / 60);
        return h + "h " + m + "m";
    }

    function renderSubsystems(subsystems) {
        var container = $("subsystem-list");
        if (!container) {
            return;
        }
        if (!subsystems || Object.keys(subsystems).length === 0) {
            container.innerHTML = '<p class="muted">No subsystem data</p>';
            return;
        }

        var html = "";
        var keys = Object.keys(subsystems);
        for (var i = 0; i < keys.length; i++) {
            var name = keys[i];
            var sub = subsystems[name];
            var subStatus = (sub && sub.status) ? sub.status : "unknown";
            var statusClass = SUBSYSTEM_OK.indexOf(subStatus) !== -1 ? "ok"
                : SUBSYSTEM_WARN.indexOf(subStatus) !== -1 ? "warn"
                : "error";

            html += '<div class="subsystem-item">'
                + '<div class="subsystem-name">' + escapeHTML(name) + '</div>'
                + '<div class="subsystem-status ' + statusClass + '">' + escapeHTML(subStatus) + '</div>'
                + '</div>';
        }
        container.innerHTML = html;
    }

    function loadHealth() {
        fetchJSON("/api/v1/health/detailed")
            .then(function (resp) {
                if (resp.success && resp.data) {
                    renderOverview(resp.data);
                    renderSubsystems(resp.data.subsystems);
                }
            })
            .catch(function () {
                setText("stat-status", "unreachable");
            });
    }

    function loadConfig() {
        fetchJSON("/api/v1/config")
            .then(function (resp) {
                var el = $("config-content");
                if (el) {
                    el.textContent = JSON.stringify(resp.data || resp, null, 2);
                }
            })
            .catch(function () {
                var el = $("config-content");
                if (el) {
                    el.textContent = "Failed to load configuration";
                }
            });
    }

    function appendLog(entry) {
        logBuffer.push(entry);
        if (logBuffer.length > 500) {
            logBuffer.shift();
        }
        renderLogs();
    }

    function renderLogs() {
        var container = $("log-entries");
        if (!container) {
            return;
        }

        var filtered = logBuffer;
        if (logFilter !== "all") {
            filtered = [];
            for (var i = 0; i < logBuffer.length; i++) {
                if (logBuffer[i].level === logFilter) {
                    filtered.push(logBuffer[i]);
                }
            }
        }

        if (filtered.length === 0) {
            container.innerHTML = '<p class="muted">No log entries</p>';
            return;
        }

        var html = "";
        var start = Math.max(0, filtered.length - 200);
        for (var i = start; i < filtered.length; i++) {
            var e = filtered[i];
            var levelClass = (e.level || "info").toLowerCase();
            html += '<div class="log-entry">'
                + '<span class="log-time">' + escapeHTML(e.time || "") + '</span>'
                + '<span class="log-level ' + levelClass + '">' + escapeHTML(levelClass.toUpperCase()) + '</span>'
                + escapeHTML(e.message || "")
                + '</div>';
        }
        container.innerHTML = html;
        container.scrollTop = container.scrollHeight;
    }

    function connectSSE() {
        var es = new EventSource("/api/v1/logs");

        es.onmessage = function (ev) {
            try {
                var entry = JSON.parse(ev.data);
                appendLog(entry);
            } catch (_) {
                // ignore malformed events
            }
        };

        es.onerror = function () {
            es.close();
            // Reconnect after 5 seconds
            setTimeout(connectSSE, 5000);
        };
    }

    function escapeHTML(str) {
        var div = document.createElement("div");
        div.appendChild(document.createTextNode(str));
        return div.innerHTML;
    }

    function init() {
        loadHealth();
        loadConfig();
        connectSSE();

        // Refresh health every 10 seconds
        setInterval(loadHealth, 10000);

        var filterEl = $("log-filter");
        if (filterEl) {
            filterEl.addEventListener("change", function () {
                logFilter = filterEl.value;
                renderLogs();
            });
        }
    }

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", init);
    } else {
        init();
    }
})();
