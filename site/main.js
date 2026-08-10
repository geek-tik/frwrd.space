document.querySelectorAll(".copy-btn").forEach(function (btn) {
  btn.addEventListener("click", function () {
    var id = btn.getAttribute("data-target");
    var el = document.getElementById(id);
    if (!el) return;

    navigator.clipboard.writeText(el.textContent.trim()).then(function () {
      var icon = btn.querySelector(".material-icons");
      icon.textContent = "check";
      setTimeout(function () {
        icon.textContent = "content_copy";
      }, 1500);
    });
  });
});

fetch("/api/v1/status")
  .then(function (r) {
    if (!r.ok) throw new Error("unavailable");
    return r.json();
  })
  .then(function (data) {
    var section = document.getElementById("status");
    var text = document.getElementById("status-text");
    var version = document.getElementById("version");

    section.hidden = false;
    text.textContent = "Сервис " + data.service + " · домен " + data.base_domain;
    if (version && data.version) {
      version.textContent = "v" + data.version;
    }
  })
  .catch(function () {});
