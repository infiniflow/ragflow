package datasets;
import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

public class GetDatasetInfo {
    public static void main(String[] args) {
      HttpClient client = HttpClient.newBuilder().build();
      HttpRequest request = HttpRequest.newBuilder()
          .uri(URI.create("http://127.0.0.1/api/v1/datasets?name=api_create_dataset"))
          .GET()
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