document.addEventListener("DOMContentLoaded", () => {
  chrome.storage.sync.get(["baseURL", "from", "auth", "sharedID"], (result) => {
    if (result.baseURL && result.sharedID && result.from && result.auth) {
      const iframeSrc = `${result.baseURL}chat/share?shared_id=${result.sharedID}&from=${result.from}&auth=${result.auth}`;
      const iframe = document.querySelector("iframe");
      iframe.src = iframeSrc;
    }
  });
  chrome.tabs.query({ active: true, currentWindow: true }, (tabs) => {
    chrome.scripting.executeScript(
      {
        target: { tabId: tabs[0].id },
        files: ["content.js"],
      },
      (results) => {
        if (results && results[0]) {
          const getHtml = document.getElementById("getHtml");
          getHtml.value = results[0].result;

        }
      }
    );
  });
});
