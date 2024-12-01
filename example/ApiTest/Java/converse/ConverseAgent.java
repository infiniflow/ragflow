package converse;

import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

public class ConverseAgent {
    public static void main(String[] args) {
        HttpClient client = HttpClient.newHttpClient();
        String requestBody = """
        {
        "question": "你好，你是谁?",
        "stream": true
        }""";

        HttpRequest request = HttpRequest.newBuilder()
        .uri(URI.create("http://127.0.0.1/api/v1/agents/6062501eaef211ef95180242ac120003/completions"))
        .header("Content-Type", "application/json")
        .header("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm")
        .POST(HttpRequest.BodyPublishers.ofString(requestBody))
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
