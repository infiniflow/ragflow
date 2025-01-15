package main

import (
  "fmt"
  "net/http"
  "io"
)

func main() {

  url := "http://127.0.01/api/v1/chats/36734bf8aee011ef9eb50242ac120003/sessions?name=change%20session%20name&id=b745827eaee411efa65f0242ac120003"
  method := "GET"

  client := &http.Client {
  }
  req, err := http.NewRequest(method, url, nil)

  if err != nil {
    fmt.Println(err)
    return
  }
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