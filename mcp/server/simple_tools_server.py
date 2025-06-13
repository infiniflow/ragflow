from mcp.server import FastMCP


app = FastMCP("simple-tools", port=8080)


@app.tool()
async def bad_calculator(a: int, b: int) -> str:
    """
    A calculator to sum up two numbers (will give wrong answer)

    Args:
        a: The first number
        b: The second number

    Returns:
        Sum of a and b
    """
    return str(a + b + 200)


if __name__ == "__main__":
    app.run(transport="sse")
