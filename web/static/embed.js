// embed.js renders the itworks.build wall inside any other page. Drop
//   <div data-itworks-wall data-limit="5"></div>
//   <script src="https://itworks.build/static/embed.js" defer></script>
// on a page and it fills the div with entries from api/entries.json.
// ES2019, no dependencies, textContent only so the host page never gets
// markup it did not write.
(function () {
  "use strict";

  var DEFAULT_FEED = "https://itworks.build/api/entries.json";
  var DEFAULT_LIMIT = 5;
  var STYLE_ID = "itw-wall-style";

  function ensureStyle() {
    if (document.getElementById(STYLE_ID)) return;
    var style = document.createElement("style");
    style.id = STYLE_ID;
    style.textContent =
      ".itw-wall{list-style:none;margin:0;padding:0;font:inherit;color:inherit}" +
      ".itw-item{display:flex;align-items:center;gap:.5em;padding:.4em 0;border-bottom:1px solid rgba(128,128,128,.25)}" +
      ".itw-item:last-child{border-bottom:none}" +
      ".itw-badge{height:1.4em;flex:none}" +
      ".itw-body{flex:1;min-width:0}" +
      ".itw-name{font-weight:600;text-decoration:none;color:inherit}" +
      ".itw-name:hover{text-decoration:underline}" +
      ".itw-summary,.itw-meta,.itw-empty,.itw-footer{margin:.15em 0 0;font-size:.85em;opacity:.8}";
    document.head.appendChild(style);
  }

  function el(tag, className) {
    var e = document.createElement(tag);
    if (className) e.className = className;
    return e;
  }

  function renderEmpty(root) {
    root.textContent = "";
    var p = el("p", "itw-empty");
    var link = el("a");
    link.href = "https://itworks.build/";
    link.textContent = "No entries yet.";
    p.appendChild(link);
    root.appendChild(p);
  }

  function textEl(tag, className, text) {
    var e = el(tag, className);
    e.textContent = text;
    return e;
  }

  function renderEntries(root, entries, builtAt) {
    root.textContent = "";
    var list = el("ol", "itw-wall");

    entries.forEach(function (entry) {
      var item = el("li", "itw-item");

      var badgeLink = el("a");
      badgeLink.href = entry.entry_url;
      var badgeImg = el("img", "itw-badge");
      badgeImg.src = entry.badge_url;
      badgeImg.alt = entry.name + " audit badge";
      badgeLink.appendChild(badgeImg);
      item.appendChild(badgeLink);

      var body = el("div", "itw-body");
      var nameLink = textEl("a", "itw-name", entry.name);
      nameLink.href = entry.entry_url;
      body.appendChild(nameLink);
      body.appendChild(textEl("p", "itw-summary", entry.summary));
      body.appendChild(
        textEl("p", "itw-meta", entry.audit_tier + " · " + entry.audit_date)
      );

      item.appendChild(body);
      list.appendChild(item);
    });

    root.appendChild(list);
    if (builtAt) {
      root.appendChild(textEl("p", "itw-footer", "as of " + builtAt));
    }
  }

  function renderWall(root) {
    var feedURL = root.getAttribute("data-feed") || DEFAULT_FEED;
    var limitAttr = parseInt(root.getAttribute("data-limit"), 10);
    var limit = isNaN(limitAttr) ? DEFAULT_LIMIT : limitAttr;

    fetch(feedURL)
      .then(function (res) {
        if (!res.ok) throw new Error("bad response");
        return res.json();
      })
      .then(function (data) {
        var entries = (data && data.entries) || [];
        if (entries.length === 0) {
          renderEmpty(root);
          return;
        }
        renderEntries(root, entries.slice(0, limit), data.built_at);
      })
      .catch(function () {
        renderEmpty(root);
      });
  }

  function init() {
    ensureStyle();
    var roots = document.querySelectorAll("[data-itworks-wall]");
    for (var i = 0; i < roots.length; i++) {
      renderWall(roots[i]);
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
