(function () {
  function initReviewCountdownButton(button) {
    if (!button || button.dataset.reviewCountdownInit === "1") {
      return;
    }
    button.dataset.reviewCountdownInit = "1";

    var label = button.dataset.reviewLabel || button.textContent.trim();
    var deadline = Number(button.dataset.reviewDeadline || "0");
    if (!label || !deadline) {
      return;
    }

    function updateLabel() {
      var seconds = Math.max(0, Math.ceil((deadline - Date.now()) / 1000));
      button.textContent = label + " (" + seconds + ")";
      return seconds;
    }

    updateLabel();
    var timer = window.setInterval(function () {
      if (!document.body.contains(button)) {
        window.clearInterval(timer);
        return;
      }
      if (updateLabel() <= 0) {
        window.clearInterval(timer);
      }
    }, 250);
  }

  function initReviewCountdownButtons(root) {
    (root || document).querySelectorAll("[data-review-deadline]").forEach(initReviewCountdownButton);
  }

  function onReady(fn) {
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", fn, { once: true });
      return;
    }
    fn();
  }

  onReady(function () {
    initReviewCountdownButtons(document);
    var observer = new MutationObserver(function () {
      initReviewCountdownButtons(document);
    });
    observer.observe(document.body, { childList: true, subtree: true });
  });
})();
