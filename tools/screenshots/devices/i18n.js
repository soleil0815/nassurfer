// SPDX-License-Identifier: AGPL-3.0-only
// Texte der Demo-Seiten folgen der Browsersprache (wie bei echten Geräten)
(function(){var de=/^de/.test(navigator.language);document.documentElement.lang=de?"de":"en";
document.querySelectorAll("[data-en]").forEach(function(e){if(!de)e.innerHTML=e.getAttribute("data-en");});
var t=document.querySelector("title");if(t&&t.dataset.en&&!de)t.textContent=t.dataset.en;})();
