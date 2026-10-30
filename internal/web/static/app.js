(function () {
  var KEY = "zypanel-theme";

  function apply(theme) {
    document.documentElement.setAttribute("data-theme", theme);
  }

  var saved = localStorage.getItem(KEY);
  if (saved === "light" || saved === "dark") {
    apply(saved);
  }

  var toggle = document.getElementById("theme-toggle");
  if (toggle) {
    toggle.addEventListener("click", function () {
      var current = document.documentElement.getAttribute("data-theme") || "dark";
      var next = current === "dark" ? "light" : "dark";
      apply(next);
      localStorage.setItem(KEY, next);
    });
  }

  document.querySelectorAll("form[data-confirm]").forEach(function (form) {
    form.addEventListener("submit", function (event) {
      var message = form.getAttribute("data-confirm") || "确认执行该操作？";
      if (!window.confirm(message)) {
        event.preventDefault();
      }
    });
  });
})();
