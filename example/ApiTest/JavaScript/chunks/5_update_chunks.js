const myHeaders = new Headers();
myHeaders.append("Content-Type", "application/json");
myHeaders.append("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm");

const raw = JSON.stringify({
  "content": "some update content",
  "important_keywords": [
    "名字由来"
  ]
});

const requestOptions = {
  method: "PUT",
  headers: myHeaders,
  body: raw,
  redirect: "follow"
};

fetch("http://127.0.0.1/api/v1/datasets/8a85ab34ad5311ef98b00242ac120003/documents/501e387aadf411ef922e0242ac120003/chunks/f4444516b61f95adfdd293173177be4a", requestOptions)
  .then((response) => response.text())
  .then((result) => console.log(result))
  .catch((error) => console.error(error));