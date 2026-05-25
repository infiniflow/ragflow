from fastapi import FastAPI, Request
app = FastAPI()
@app.post("/")
async def echo(request: Request):
    body = await request.body()
    return body
if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)