(function () {
  var busy = {};
  var stats = { sites: [], shared: { mem: 0, cpu: 0 } };
  var sites = [];

  function $(id) { return document.getElementById(id); }

  function fmtMem(b) {
    if (!b) return "-";
    if (b >= 1 << 30) return (b / (1 << 30)).toFixed(1) + " GiB";
    if (b >= 1 << 20) return Math.round(b / (1 << 20)) + " MiB";
    return Math.round(b / 1024) + " KiB";
  }

  function showMsg(text) {
    var m = $("msg");
    m.textContent = text || "";
    m.hidden = !text;
  }

  function api(method, path, body) {
    var opts = { method: method, headers: {} };
    if (method === "POST") {
      opts.headers["X-MD"] = "1";
      opts.headers["Content-Type"] = "application/json";
      opts.body = JSON.stringify(body || {});
    }
    return fetch(path, opts).then(function (r) {
      setOffline(false);
      return r.json().catch(function () { return {}; }).then(function (j) {
        if (!r.ok) throw new Error(j.error || r.statusText);
        return j;
      });
    }, function (e) {
      setOffline(true);
      e.offline = true;
      throw e;
    });
  }

  function setOffline(v) {
    $("offline").hidden = !v;
  }

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  function link(label, href) {
    var a = el("a", "", label);
    a.href = href;
    a.target = "_blank";
    a.rel = "noopener";
    return a;
  }

  function statOf(name) {
    for (var i = 0; i < stats.sites.length; i++) if (stats.sites[i].name === name) return stats.sites[i];
    return null;
  }

  function act(name, action) {
    busy[name] = action;
    render();
    api("POST", "/api/sites/" + encodeURIComponent(name) + "/" + action)
      .catch(function (e) { if (!e.offline) showMsg(name + " " + action + ": " + e.message); })
      .then(function () { delete busy[name]; refreshSites(); });
  }

  function badge(parent, text, title) {
    var b = el("span", "badge", text);
    b.title = title;
    parent.appendChild(b);
  }

  function actionBtn(label, name, action, disabled) {
    var b = el("button", "", label);
    b.disabled = disabled;
    b.onclick = function () { act(name, action); };
    return b;
  }

  function render() {
    var tb = document.querySelector("#sites tbody");
    tb.textContent = "";
    var totMem = 0, totCpu = 0;
    sites.forEach(function (s) {
      var tr = el("tr");
      tr.appendChild(el("td", "", s.name));
      var label = busy[s.name] ? busy[s.name] + "..." : s.status === "ddev" ? "running (ddev)" : s.status;
      var cls = busy[s.name] ? "busy" : s.status;
      tr.appendChild(el("td", "st " + cls, label));
      var links = el("td");
      if (s.status !== "stopped") {
        ["storefront", "admin", "mailpit", "xhgui", "phpmyadmin"].forEach(function (k) {
          if (s.urls && s.urls[k]) links.appendChild(link(k, s.urls[k]));
        });
      }
      tr.appendChild(links);
      tr.appendChild(el("td", "", s.php || "-"));
      tr.appendChild(el("td", "", s.db || "-"));
      tr.appendChild(el("td", "", s.search || "-"));
      var u = statOf(s.name);
      if (u) { totMem += u.mem; totCpu += u.cpu; }
      tr.appendChild(el("td", "num", u ? fmtMem(u.mem) : "-"));
      tr.appendChild(el("td", "num", u ? u.cpu.toFixed(1) + "%" : "-"));
      var ac = el("td", "actions");
      var wait = !!busy[s.name];
      var up = s.status !== "stopped";
      ac.appendChild(actionBtn("Up", s.name, "up", wait || up || !!s.blocked));
      ac.appendChild(actionBtn("Stop", s.name, "stop", wait || !up));
      ac.appendChild(actionBtn("Restart", s.name, "restart", wait || !up));
      if (s.blocked) badge(ac, "chưa hỗ trợ md", s.blocked);
      else if (s.note) badge(ac, "chưa hỗ trợ md", s.note);
      if (s.status === "stopped" && s.source === "ddev" && !s.blocked) badge(ac, "md migrate", "Run md migrate in the site root to move this site to md");
      tr.appendChild(ac);
      tb.appendChild(tr);
    });
    totMem += stats.shared.mem || 0;
    totCpu += stats.shared.cpu || 0;
    $("total-mem").textContent = fmtMem(totMem);
    $("total-cpu").textContent = totCpu.toFixed(1) + "%";
    $("shared").textContent = fmtMem(stats.shared.mem) + " / " + (stats.shared.cpu || 0).toFixed(1) + "%";
  }

  function refreshSites() {
    return api("GET", "/api/sites").then(function (s) { sites = s; render(); }).catch(function (e) { if (!e.offline) showMsg(e.message); });
  }

  function refreshStats() {
    return api("GET", "/api/stats").then(function (s) { stats = s; render(); }).catch(function () {});
  }

  function tiles(id, items) {
    var g = $(id), groups = {}, order = [];
    g.textContent = "";
    items.forEach(function (it) {
      if (!groups[it.group]) { groups[it.group] = []; order.push(it.group); }
      groups[it.group].push(it);
    });
    order.forEach(function (name) {
      var t = el("div", "tile");
      t.appendChild(el("div", "tile-name", name));
      groups[name].forEach(function (it) {
        var row = el("div", it.idle ? "tile-row idle" : "tile-row");
        row.appendChild(el("span", "", it.name));
        row.appendChild(el("b", "", it.size));
        row.appendChild(el("small", "", it.meta));
        t.appendChild(row);
      });
      g.appendChild(t);
    });
  }

  function refreshDisk() {
    api("GET", "/api/disk").then(function (d) {
      tiles("images", d.images.map(function (i) {
        var used = +i.containers ? (i.used_by ? "used by " + i.used_by : "in use: " + i.containers) : "unused";
        return { group: i.repo === "<none>" ? "untagged (old builds)" : i.repo, name: i.tag, size: i.size, meta: used, idle: !+i.containers };
      }));
      tiles("volumes", d.volumes.map(function (v) {
        return { group: v.site || "no site", name: v.name, size: v.size, meta: "", idle: !v.site };
      }));
    }).catch(function (e) { if (!e.offline) showMsg(e.message); });
  }

  function showTab(name) {
    ["sites", "disk"].forEach(function (t) { $("tab-" + t).hidden = t !== name; });
    document.querySelectorAll(".tab").forEach(function (b) { b.classList.toggle("active", b.dataset.tab === name); });
    if (name === "disk") refreshDisk();
  }

  document.querySelectorAll(".tab").forEach(function (b) { b.onclick = function () { showTab(b.dataset.tab); }; });

  $("poweroff").onclick = function () {
    if (!window.confirm("Stop every running site and the shared stack?")) return;
    api("POST", "/api/poweroff").catch(function (e) { if (!e.offline) showMsg("poweroff: " + e.message); }).then(refreshSites);
  };

  $("plan").onclick = function () {
    api("GET", "/api/clean").then(function (p) {
      var lines = p.items.map(function (i) { return i.Kind + "  " + i.Name + "  " + i.Size; });
      if (p.orphans.length) {
        lines.push("", "Orphan DB volumes (remove by hand):");
        p.orphans.forEach(function (i) { lines.push("  docker volume rm " + i.Name); });
      }
      $("plan-out").textContent = lines.length ? lines.join("\n") : "";
      if (!p.items.length) showMsg("Nothing to clean");
    }).catch(function (e) { if (!e.offline) showMsg(e.message); });
  };

  $("clean").onclick = function () {
    var c = $("confirm").value.trim();
    if (c !== "yes") { showMsg("Type yes to confirm cleanup"); return; }
    api("POST", "/api/clean", { confirm: c }).then(function () {
      $("confirm").value = "";
      $("plan-out").textContent = "";
      showMsg("");
      refreshDisk();
    }).catch(function (e) { if (!e.offline) showMsg("clean: " + e.message); });
  };

  var pollStats = null, pollSites = null;

  function startPolling() {
    if (pollStats) return;
    refreshStats();
    pollStats = setInterval(refreshStats, 3000);
    pollSites = setInterval(refreshSites, 5000);
  }

  function stopPolling() {
    clearInterval(pollStats);
    clearInterval(pollSites);
    pollStats = pollSites = null;
  }

  function listen() {
    var es = new EventSource("/api/events");
    es.onopen = function () { stopPolling(); setOffline(false); };
    es.onerror = function () { setOffline(true); startPolling(); };
    es.addEventListener("sites", function (e) { sites = JSON.parse(e.data); render(); });
    es.addEventListener("stats", function (e) { stats = JSON.parse(e.data); render(); });
  }

  refreshSites();
  if (window.EventSource) listen(); else startPolling();
})();
