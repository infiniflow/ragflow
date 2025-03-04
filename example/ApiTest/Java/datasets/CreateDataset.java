package datasets;
import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;

public class CreateDataset {
    public static void main(String[] args) {
        String jsonBody = """
            {
                "name": "api_create_dataset3",
                "language": "Chinese", 
                "embedding_model": "text-embedding-v3"
            }""";

        HttpClient client = HttpClient.newBuilder()
            .version(HttpClient.Version.HTTP_1_1)
            .connectTimeout(Duration.ofSeconds(10))
            .build();

        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create("http://127.0.0.1/api/v1/datasets"))
            .POST(HttpRequest.BodyPublishers.ofString(jsonBody))
            .header("Content-Type", "application/json")
            .header("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm")
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