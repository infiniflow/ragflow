# Start an AI chat

Knowledge base, hallucination-free chat, and file management are three pillars of RAGFlow. Conversations in RAGFlow are based on a particular knowledge base or multiple knowledge bases. Once you have created your knowledge base and finished file parsing, you can go ahead and start an AI conversation. 

1. Click the **Chat** tab in the middle top of the mage **>** **Create an assistant** to show the **Chat Configuration** dialogue *of your next dialogue*.

   > RAGFlow offer the flexibility of choosing a different chat model for each dialogue, while allowing you to set the default models in **System Model Settings**.

2. Update **Assistant Setting**: 

   - Name your assistant and specify your knowledge bases.
   - **Empty response**:
     - If you wish to *confine* RAGFlow's answers to your knowledge bases, leave a response here. Then when it doesn't retrieve an answer, it *uniformly* responds with what you set here. 
     - If you wish RAGFlow to *improvise* when it doesn't retrieve an answer from your knowledge bases, leave it blank, which may give rise to hallucinations. 

3. Update **Prompt Engine** or leave it as is for the beginning.

4. Update **Model Setting**.

5. RAGFlow also offers conversation APIs. Hover over your dialogue **>** **Chat Bot API** to integrate RAGFlow's chat capabilities into your applications:

   ![chatbot api](https://github.com/infiniflow/ragflow/assets/93570324/fec23715-f9af-4ac2-81e5-942c5035c5e6)

6. Now, let's start the show:

   ![question1](https://github.com/infiniflow/ragflow/assets/93570324/bb72dd67-b35e-4b2a-87e9-4e4edbd6e677)

   ![question2](https://github.com/infiniflow/ragflow/assets/93570324/7cc585ae-88d0-4aa2-817d-0370b2ad7230)
