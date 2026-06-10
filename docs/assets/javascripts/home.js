document.addEventListener("DOMContentLoaded", function () {
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
