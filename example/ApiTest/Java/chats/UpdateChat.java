package chats;

import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

public class UpdateChat {
    public static void main(String[] args) {
        HttpClient client = HttpClient.newHttpClient();
        String requestBody = """
        {
            "name":"update chat name",
            "llm": {"model":"moonshot-v1-8k"}
        }""";

        HttpRequest request = HttpRequest.newBuilder()
        .uri(URI.create("http://127.0.0.1/api/v1/chats/36734bf8aee011ef9eb50242ac120003"))
        .header("Content-Type", "application/json")
        .header("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm")
        .PUT(HttpRequest.BodyPublishers.ofString(requestBody))
        .build();

        try {
            HttpResponse<String> response = client.send(request, HttpResponse.BodyHandlers.ofString());
            System.out.println("Response Code: " + response.statusCode());
            System.out.println("Response Body: " + response.body());
        } catch (IOException | InterruptedException e) {
            e.printStackTrace();
        }
    }
}
