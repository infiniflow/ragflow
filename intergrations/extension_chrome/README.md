# Chrome Extension

chrome-extension/
│
├── manifest.json       # File cấu hình chính của tiện ích
├── popup.html          # Giao diện chính của tiện ích
├── popup.js            # Script cho giao diện chính
├── background.js       # Script chạy nền của tiện ích
├── content.js          # Script thao tác với trang web
├── styles/
│   └── popup.css       # File CSS cho popup
├── icons/
│   ├── icon16.png      # Icon kích thước 16x16
│   ├── icon48.png      # Icon kích thước 48x48
│   └── icon128.png     # Icon kích thước 128x128
├── assets/
│   └── ...             # Thư mục chứa các tài nguyên khác (ảnh, fonts, v.v.)
├── scripts/
│   ├── utils.js        # File chứa các hàm tiện ích
│   └── api.js          # File chứa logic gọi API
└── README.md           # Hướng dẫn sử dụng và cài đặt tiện ích


## Cách cài đặt
1. Mở **chrome://extensions/**.
2. Bật **Developer mode**.
3. Nhấn **Load unpacked** và chọn thư mục dự án.

## Tính năng
- Tương tác với trang web.
- Chạy nền để xử lý logic.

## Cách sử dụng
1. Click vào biểu tượng tiện ích trên thanh công cụ.
2. Làm theo hướng dẫn trong giao diện.
