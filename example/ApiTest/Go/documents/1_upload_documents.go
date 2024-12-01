package main

import (
  "fmt"
  "bytes"
  "mime/multipart"
  "os"
  "path/filepath"
  "net/http"
  "io"
)

func main() {

  url := "http://127.0.0.1/api/v1/datasets/8a85ab34ad5311ef98b00242ac120003/documents"
  method := "POST"

  payload := &bytes.Buffer{}
  writer := multipart.NewWriter(payload)
  file, errFile1 := os.Open("D:/ragflow/hd.txt")
  defer file.Close()
  part1,
         errFile1 := writer.CreateFormFile("file",filepath.Base("D:/ragflow/hd.txt"))
  _, errFile1 = io.Copy(part1, file)
  if errFile1 != nil {
    fmt.Println(errFile1)
    return
  }
  file, errFile2 := os.Open("D:/ragflow/测试.txt")
  defer file.Close()
  part2,
         errFile2 := writer.CreateFormFile("file",filepath.Base("D:/ragflow/测试.txt"))
  _, errFile2 = io.Copy(part2, file)
  if errFile2 != nil {
    fmt.Println(errFile2)
    return
  }
  err := writer.Close()
  if err != nil {
    fmt.Println(err)
    return
  }


  client := &http.Client {
  }
  req, err := http.NewRequest(method, url, payload)

  if err != nil {
    fmt.Println(err)
    return
  }
  req.Header.Add("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm")

  req.Header.Set("Content-Type", writer.FormDataContentType())
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