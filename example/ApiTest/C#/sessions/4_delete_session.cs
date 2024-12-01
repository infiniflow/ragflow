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
            var request = new HttpRequestMessage(HttpMethod.Delete, "http://127.0.01/api/v1/chats/36734bf8aee011ef9eb50242ac120003/sessions");
            request.Headers.Add("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm");
            var content = new StringContent("\n     {\n          \"ids\": [\"b745827eaee411efa65f0242ac120003\"]\n     }", null, "application/json");
            request.Content = content;
            var response = await client.SendAsync(request);
            response.EnsureSuccessStatusCode();
            Console.WriteLine(await response.Content.ReadAsStringAsync());

        }
    }
}
