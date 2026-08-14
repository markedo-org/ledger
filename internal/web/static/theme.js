(function () {
  var key = "ledger-theme";
  var root = document.documentElement;
  var btn = document.getElementById("theme-flip");
  if (!btn) return;

  function stored() {
    try {
      return localStorage.getItem(key);
    } catch (e) {
      return null;
    }
  }

  function save(v) {
    try {
      if (!v) localStorage.removeItem(key);
      else localStorage.setItem(key, v);
    } catch (e) {}
  }

  function osDark() {
    return window.matchMedia("(prefers-color-scheme: dark)").matches;
  }

  function effective() {
    var s = stored();
    if (s === "light" || s === "dark") return s;
    return osDark() ? "dark" : "light";
  }

  function apply() {
    var s = stored();
    if (s === "light" || s === "dark") root.setAttribute("data-theme", s);
    else root.removeAttribute("data-theme");
    var cur = effective();
    btn.dataset.theme = cur;
    var next = cur === "dark" ? "light" : "dark";
    var label = "Switch to " + next + " appearance";
    btn.setAttribute("aria-label", label);
    btn.setAttribute("title", label);
  }

  btn.addEventListener("click", function () {
    var want = effective() === "dark" ? "light" : "dark";
    var os = osDark() ? "dark" : "light";
    save(want === os ? "" : want);
    apply();
  });

  apply();
  try {
    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", apply);
  } catch (e) {}
})();
