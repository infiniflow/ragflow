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
            var request = new HttpRequestMessage(HttpMethod.Delete, "http://127.0.0.1/api/v1/chats");
            request.Headers.Add("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm");
            var content = new StringContent("\n     {\n          \"ids\": [\"b8f7957aabd411efafbd0242ac120006\", \"b056fb6eabd811efb6000242ac120006\"]\n     }", null, "application/json");
            request.Content = content;
            var response = await client.SendAsync(request);
            response.EnsureSuccessStatusCode();
            Console.WriteLine(await response.Content.ReadAsStringAsync());
        }
    }
}
