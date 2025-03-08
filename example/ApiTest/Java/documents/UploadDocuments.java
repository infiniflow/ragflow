package documents;

import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpRequest.BodyPublishers;
import java.net.http.HttpResponse;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;

public class UploadDocuments {
    public static void main(String[] args) {
        try {
            // 创建 HttpClient 实例
            HttpClient client = HttpClient.newHttpClient();

            // 定义文件路径
            Path file1 = Paths.get("D:/ragflow/hd.txt");
            Path file2 = Paths.get("D:/ragflow/测试.txt");

            // 构建 multipart/form-data 请求体
            String boundary = "----Boundary" + System.currentTimeMillis();
            String contentType = "multipart/form-data; boundary=" + boundary;

            String multipartBody = """
                    --%1$s
                    Content-Disposition: form-data; name="file"; filename="%2$s"
                    Content-Type: application/octet-stream

                    %3$s
                    --%1$s
                    Content-Disposition: form-data; name="file"; filename="%4$s"
                    Content-Type: application/octet-stream

                    %5$s
                    --%1$s--
                    """.formatted(
                    boundary,
                    file1.getFileName(),
                    new String(Files.readAllBytes(file1)), // 读取文件内容
                    file2.getFileName(),
                    new String(Files.readAllBytes(file2)) // 读取文件内容
            );

            // 创建 HttpRequest
            HttpRequest request = HttpRequest.newBuilder()
                    .uri(URI.create("http://127.0.0.1/api/v1/datasets/8a85ab34ad5311ef98b00242ac120003/documents"))
                    .header("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm")
                    .header("Content-Type", contentType)
                    .POST(BodyPublishers.ofString(multipartBody))
                    .build();

            // 执行请求
            HttpResponse<String> response = client.send(request, HttpResponse.BodyHandlers.ofString());

            // 打印响应
            System.out.println("Response Code: " + response.statusCode());
            System.out.println("Response Body: " + response.body());
        } catch (IOException | InterruptedException e) {
            // 捕获并处理 IOException 和 InterruptedException
            e.printStackTrace();
        }
    }
}
