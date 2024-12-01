const myHeaders = new Headers();
myHeaders.append("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm");

const requestOptions = {
  method: "GET",
  headers: myHeaders,
  redirect: "follow"
};

fetch("http://127.0.0.1/api/v1/datasets/8a85ab34ad5311ef98b00242ac120003/documents/985440fead5d11efadb40242ac120003", requestOptions)
  .then((response) => response.text())
  .then((result) => console.log(result))
  .catch((error) => console.error(error));