package main

import (
  "fmt"
  "strings"
  "net/http"
  "io"
)

func main() {

  url := "http://127.0.0.1/api/v1/datasets/919f28e0ac9b11efabe50242ac120003"
  method := "PUT"

  payload := strings.NewReader(`
     {
          "name": "api_updated_dataset",
          "chunk_method":"manual",
          "embedding_model":"embedding-3"
     }`)

  client := &http.Client {
  }
  req, err := http.NewRequest(method, url, payload)

  if err != nil {
    fmt.Println(err)
    return
  }
  req.Header.Add("Content-Type", "application/json")
  req.Header.Add("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm")

  res, err := client.Do(req)
  if err != nil {
    fmt.Println(err)
    return
  }
  defer res.Body.Close()

  body, err := io.ReadAll(res.Body)
  if err != nil {
    fmt.Println(err)
    return
  }
  fmt.Println(string(body))
}