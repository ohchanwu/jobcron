(function () {
  "use strict";

  var storageKey = "jobcronBriefingSeenAt";
  var dot = document.querySelector(".briefing-dot");
  if (!dot) return;

  fetch("/api/briefing-status", { headers: { Accept: "application/json" } })
    .then(function (response) {
      if (!response.ok) throw new Error("briefing status " + response.status);
      return response.json();
    })
    .then(function (status) {
      if (status.profile_required || !status.latest) {
        dot.hidden = true;
        return;
      }

      if (window.location.pathname === "/briefing") {
        localStorage.setItem(storageKey, status.latest);
        dot.hidden = true;
        return;
      }

      var seenAt = localStorage.getItem(storageKey);
      dot.hidden = Boolean(seenAt && Date.parse(seenAt) >= Date.parse(status.latest));
    })
    .catch(function () {
      dot.hidden = true;
    });
})();
