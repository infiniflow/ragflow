const myHeaders = new Headers();
myHeaders.append("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm");

const formdata = new FormData();
formdata.append("file", fileInput.files[0], "D:/ragflow/hd.txt");
formdata.append("file", fileInput.files[0], "D:/ragflow/测试.txt");

const requestOptions = {
  method: "POST",
  headers: myHeaders,
  body: formdata,
  redirect: "follow"
};

fetch("http://127.0.0.1/api/v1/datasets/8a85ab34ad5311ef98b00242ac120003/documents", requestOptions)
  .then((response) => response.text())
  .then((result) => console.log(result))
  .catch((error) => console.error(error));