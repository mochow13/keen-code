document.addEventListener("DOMContentLoaded", () => {
  const logo = document.querySelector("a.md-logo");
  const title = document.querySelector(".md-header__title");
  if (!logo || !title) return;
  title.style.cursor = "pointer";
  title.setAttribute("role", "link");
  title.setAttribute("tabindex", "0");
  title.setAttribute("aria-label", "Go to homepage");
  const goHome = () => logo.click();
  title.addEventListener("click", goHome);
  title.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      goHome();
    }
  });
});
