(function () {
  const now = new Date();
  const footerYear = document.getElementById('footerYear');
  if (footerYear) {
    footerYear.textContent = `© ${now.getFullYear()} Qazna Foundation`;
  }

  const hero = document.getElementById('hero');
  const closeBtn = hero?.querySelector('.qz-hero-close');
  if (hero && closeBtn) {
    closeBtn.addEventListener('click', () => {
      hero.classList.add('qz-hero-hidden');
      setTimeout(() => hero.remove(), 300);
    });
  }
})();
