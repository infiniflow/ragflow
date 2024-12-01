package chunks;

import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

public class RetrieveChunk {
    public static void main(String[] args) { 
        HttpClient client = HttpClient.newHttpClient();
        String requestBody = """
        {
            "question": "some questions?",
            "dataset_ids": ["8a85ab34ad5311ef98b00242ac120003"],
            "document_ids": ["501e387aadf411ef922e0242ac120003"]
        }""";

        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create("http://127.0.0.1/api/v1/retrieval"))
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
