const myHeaders = new Headers();
myHeaders.append("Content-Type", "application/json");
myHeaders.append("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm");

const raw = JSON.stringify({
  "name": "api_create_dataset",
  "language": "Chinese",
  "embedding_model": "text-embedding-v3"
});

const requestOptions = {
  method: "POST",
  headers: myHeaders,
  body: raw,
  redirect: "follow"
};

fetch("http://127.0.0.1/api/v1/datasets", requestOptions) 
  .then((response) => response.text()) 
  .then((result) => console.log(result))
  .catch((error) => console.error(error));