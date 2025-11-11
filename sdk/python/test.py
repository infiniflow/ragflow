from .ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="ragflow-LvXXscstl-J10l83mjj0fLR_WC6yyIqierI7YlQCyrA", base_url="http://localhost:9222")
assistant = rag_object.list_chats()
assistant = assistant[0]
session = assistant.create_session()    

print("\n==================== Miss R =====================\n")
print("Hello. What can I do for you?")

while True:
    question = input("\n==================== User =====================\n> ")
    print("\n==================== Miss R =====================\n")

    for ans in session.ask(question, stream=True):
        print(ans.content, end='', flush=True)