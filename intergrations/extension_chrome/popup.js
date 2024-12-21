document.addEventListener('DOMContentLoaded', () => {
    // Lấy các giá trị đã lưu trong storage
    chrome.storage.sync.get(['baseURL', 'from', 'auth', 'sharedID'], (result) => {
      if (result.baseURL) {
        document.getElementById('base-url').value = result.baseURL;
      }
      if (result.from) {
        document.getElementById('from').value = result.from;
      }
      if (result.auth) {
        document.getElementById('auth').value = result.auth;
      }
      if (result.sharedID) {
        document.getElementById('shared-id').value = result.sharedID;
      }
  
      // Nếu đã có thiết lập, mở giao diện iframe
      if (result.baseURL && result.sharedID && result.from && result.auth) {
        const iframeSrc = `${result.baseURL}/chat/share?shared_id=${result.sharedID}&from=${result.from}&auth=${result.auth}`;
        const iframe = document.querySelector('iframe');
        iframe.src = iframeSrc;
      }
    });
  
    document.getElementById('save-config').addEventListener('click', () => {
      const baseURL = document.getElementById('base-url').value;
      const from = document.getElementById('from').value;
      const auth = document.getElementById('auth').value;
      const sharedID = document.getElementById('shared-id').value;
  
      // Lưu trữ các giá trị thiết lập vào storage của tiện ích
      chrome.storage.sync.set({
        baseURL: baseURL,
        from: from,
        auth: auth,
        sharedID: sharedID
      }, () => {
        console.log('Các thiết lập đã được lưu.');
  
        // Mở giao diện iframe với các thiết lập đã lưu
        const iframeSrc = `${baseURL}/chat/share?shared_id=${sharedID}&from=${from}&auth=${auth}`;
        const iframe = document.querySelector('iframe');
        iframe.src = iframeSrc;
      });
    });
  });
  