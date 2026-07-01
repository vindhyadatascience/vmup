document.addEventListener("DOMContentLoaded", function () {
  // Platform-aware install command: swap the shown command between the
  // macOS/Linux and Windows variants, defaulting to the visitor's OS.
  var cmd = document.getElementById("vmup-install-cmd");
  var tabs = Array.prototype.slice.call(
    document.querySelectorAll(".vmup-install-tab")
  );

  if (cmd && tabs.length) {
    var setOS = function (os) {
      var next = cmd.getAttribute("data-cmd-" + os);
      if (next) {
        cmd.textContent = next;
      }
      tabs.forEach(function (tab) {
        tab.setAttribute("aria-selected", tab.dataset.os === os ? "true" : "false");
      });
    };

    tabs.forEach(function (tab) {
      tab.addEventListener("click", function () {
        setOS(tab.dataset.os);
      });
    });

    var ua = (navigator.userAgent || "") + " " + (navigator.platform || "");
    setOS(/win/i.test(ua) ? "windows" : "unix");
  }

  // Copy-to-clipboard for the install command.
  var btn = document.querySelector(".vmup-copy");
  if (!btn) return;
  btn.addEventListener("click", function () {
    var target = document.getElementById(btn.dataset.copyTarget);
    if (!target) return;
    navigator.clipboard.writeText(target.textContent.trim()).then(function () {
      btn.classList.add("vmup-copied");
      setTimeout(function () {
        btn.classList.remove("vmup-copied");
      }, 1500);
    });
  });
});
