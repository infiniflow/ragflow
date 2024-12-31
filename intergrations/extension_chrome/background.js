chrome.runtime.onInstalled.addListener(() => {
  console.log("Tiện ích đã được cài đặt!");
});

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message.action === "PAGE_INFO") {
    console.log( message);


    chrome.storage.local.set({ pageInfo: message }, () => {
      console.log("Page info saved to local storage.");
    });

    // Send a response to the content script
    sendResponse({ status: "success", message: "Page info received and processed." });
  }
});