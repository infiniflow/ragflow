using System;
using System.Collections.Generic;
using System.Linq;
using System.Net.Http;
using System.Text;
using System.Threading.Tasks;

namespace RagflowAPI
{
    internal class Program
    {
        static async Task Main(string[] args)
        {
            var client = new HttpClient();
            var request = new HttpRequestMessage(HttpMethod.Post, "http://127.0.0.1/api/v1/datasets");
            request.Headers.Add("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm");
            var content = new StringContent("{\r\n      \"name\": \"api_create_dataset\" ,\r\n      \"language\": \"Chinese\",\r\n      \"embedding_model\":\"text-embedding-v3\"\r\n      }", null, "application/json");
            request.Content = content;
            var response = await client.SendAsync(request);
            response.EnsureSuccessStatusCode();
            Console.WriteLine(await response.Content.ReadAsStringAsync());

        }
    }
}
