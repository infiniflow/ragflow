 using System;
using System.IO;
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
            var request = new HttpRequestMessage(HttpMethod.Post, "http://127.0.0.1/api/v1/datasets/8a85ab34ad5311ef98b00242ac120003/documents");
            request.Headers.Add("Authorization", "Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm");
            var content = new MultipartFormDataContent();
            content.Add(new StreamContent(File.OpenRead("D:/ragflow/hd.txt")), "file", "hd.txt");
            content.Add(new StreamContent(File.OpenRead("D:/ragflow/测试.txt")), "file", "测试.txt");
            request.Content = content;
            var response = await client.SendAsync(request);
            response.EnsureSuccessStatusCode();
            Console.WriteLine(await response.Content.ReadAsStringAsync());

        }
    }
}
