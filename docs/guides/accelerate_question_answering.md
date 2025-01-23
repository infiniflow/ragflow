---
sidebar_position: 2
slug: /accelerate_question_answering
---

# Accelerate document indexing and retrieval

A checklist to speed up document parsing and question answering.

---

Please note that several of your settings may *significantly* increase the time required for document parsing and retrieval. If you often find that document parsing and question answering are time-consuming, here is a checklist to consider:

1. Use GPU to reduce embedding time.
2. On the configuration page for your knowledge base, disabling **Use RAGTOR to enhance retrieval** will significantly reduce retrieval time.
3. When chatting with your chat assistant, click the light bubble icon above the *current* dialogue and scroll down the popup window to view the time taken for each task.  
   ![enlighten](https://github.com/user-attachments/assets/fedfa2ee-21a7-451b-be66-20125619923c)
4. In the **Prompt Engine** tab of your **Chat Configuration** dialogue, disabling **Multi-turn optimization** will reduce the time required to get an answer from the LLM.
5. In the **Prompt Engine** tab of your **Chat Configuration** dialogue, leaving the **Rerank model** field empty will significantly decrease retrieval time.
