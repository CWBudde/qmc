/* Keep the latest live announcement without flooding assistive technology. */
(function () {
  "use strict";
  function announcer(region, interval) {
    let last = -Infinity;
    let timer = null;
    let latest = "";
    function publish() {
      timer = null;
      last = performance.now();
      region.textContent = latest;
    }
    window.addEventListener("pagehide", () => clearTimeout(timer));
    return (message) => {
      latest = message;
      clearTimeout(timer);
      const wait = interval - (performance.now() - last);
      if (wait <= 0) publish();
      else timer = setTimeout(publish, wait);
    };
  }
  window.Accessibility = { announcer };
})();
