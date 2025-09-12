document.addEventListener("DOMContentLoaded", () => {
    
  chrome.storage.sync.get(["baseURL", "from", "auth", "sharedID"], (result) => {
    if (result.baseURL) {
      document.getElementById("base-url").value = result.baseURL;
    }
    if (result.from) {
      document.getElementById("from").value = result.from;
    }
    if (result.auth) {
      document.getElementById("auth").value = result.auth;
    }
    if (result.sharedID) {
      document.getElementById("shared-id").value = result.sharedID;
    }
  });

  document.getElementById("save-config").addEventListener("click", () => {
    const baseURL = document.getElementById("base-url").value;
    const from = document.getElementById("from").value;
    const auth = document.getElementById("auth").value;
    const sharedID = document.getElementById("shared-id").value;

    chrome.storage.sync.set(
      {
        baseURL: baseURL,
        from: from,
        auth: auth,
        sharedID: sharedID,
      },
      () => {
        alert("Successfully saved");
      }
    );
  });
});
