"""
You may use this script to test features related to the Python SDK.

Execution instructions are as follows:

>>> python test.py --api-key yourkeys --base-url YourBaseUrl --agent-id YourAgentID

Of course, if you run the script directly without passing relevant parameters via the terminal

the default values will be used.

Modifying the default values to launch tests is also an available option.

----------------------------------------------------------------------------------

Supported environment variables (higher priority than built-in defaults):
- RAGFLOW_API_KEY: RAGFlow access API key
- RAGFLOW_BASE_URL: RAGFlow service address
- RAGFLOW_AGENT_ID: Target agent unique identifier

You can configure the environment variables with the following commands on Linux/Mac:

>>> export RAGFLOW_API_KEY=YourApiKey
>>> export RAGFLOW_BASE_URL=YourBaseUrl
>>> export RAGFLOW_AGENT_ID=YourAgentID

For Windows, use the commands below (PowerShell):

>>> $env:RAGFLOW_API_KEY=YourApiKey
>>> $env:RAGFLOW_BASE_URL=YourBaseUrl
>>> $env:RAGFLOW_AGENT_ID=YourAgentID


Mixed usage is also supported.
Example: The API key is already set via environment variable, and you only want to temporarily override the agent ID:

>>> python test.py --agent-id YourAgentID

"""

from argparse import (

    Namespace,
    ArgumentParser)
# Add current script directory to Python module search path to locate ragflow_sdk package
import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from ragflow_sdk import RAGFlow

def parse_args()->"Namespace":
    """
    Parse Script Input Parameters

    Args:
        None
    Returns:
        Namespace
    
    """
    parser = ArgumentParser(description="RAGFlow Agent Test Script")
    parser.add_argument("--api-key",
                        type=str,
                        default=os.getenv("RAGFLOW_API_KEY", ""),
                        help="RAGFlow API Key; read from env RAGFLOW_API_KEY if not specified (required)")
    parser.add_argument("--base-url",
                        type=str,
                        default=os.getenv("RAGFLOW_BASE_URL", "http://localhost:9222"),
                        help="RAGFlow service base url; read from env RAGFLOW_BASE_URL if not specified")
    parser.add_argument("--agent-id",
                        type=str,
                        default=os.getenv("RAGFLOW_AGENT_ID", "b0bc46e43dfc11f1b4ff84ba59bc54d9"),
                        help="Target Agent ID; read from env RAGFLOW_AGENT_ID if not specified")
    return parser.parse_args()

def main():
    """
    Core Test Functionality

    Args:
        None
    
    Returns:
        None
    """
    args = parse_args()
    if not args.api_key.strip():
        raise SystemExit("Error: API key is required. Pass it via --api-key argument or set environment variable RAGFLOW_API_KEY.")
    
    rag_object = RAGFlow(api_key=args.api_key, base_url=args.base_url)
    assistant = rag_object.get_agent(args.agent_id)
    session = assistant.create_session()

    print("\n==================== Miss R =====================\n")
    print("Hello. What can I do for you?")

    while True:
        question = input("\n==================== User =====================\n> ")
        print("\n==================== Miss R =====================\n")

        cont = ""
        for ans in session.ask(question, stream=True):
            print(ans.content[len(cont) :], end="", flush=True)
            cont = ans.content

if __name__ == "__main__":
    main()
