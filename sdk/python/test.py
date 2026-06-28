from .ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="ragflow-FDfRECsXDRagsKPxb_EfZdDPcmngavSgYEzbU_Blgq4", base_url="http://localhost:9222")
assistant = rag_object.get_agent("b0bc46e43dfc11f1b4ff84ba59bc54d9")
session = assistant.create_session()    

print("\n==================== Miss R =====================\n")
print("Hello. What can I do for you?")

while True:
    question = input("\n==================== User =====================\n> ")
    print("\n==================== Miss R =====================\n")
    
    cont = ""
    for ans in session.ask(question, stream=True):
        print(ans.content[len(cont):], end='', flush=True)
        cont = ans.content
