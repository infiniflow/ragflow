---
sidebar_position: 2
sidebar_label: "Chat application permissions"
---

## Chat application permissions

Chat application permissions control viewing, configuration, creation, deletion, generation, and retrieval operations for chat assistants.

### Chat management

| Operation | No permission | Read | Write | Manage | Owner |
| --- | --- | --- | --- | --- | --- |
| Get chat list | X | Y | Y | Y | Y |
| Get chat details | X | Y | Y | Y | Y |
| Full update of chat configuration | X | X | X | Y | Y |
| Partial update of chat configuration | X | X | X | Y | Y |
| Create chat application configuration | X | X | X | X | Y |
| Batch delete chats through `ids`, `delete_all`, or request-body `chat_id` | X | X | X | X | Y |
| Delete a single chat | X | X | X | X | Y |

### Sessions and messages

| Operation | No permission | Read | Write | Manage | Owner |
| --- | --- | --- | --- | --- | --- |
| Create session | X | Y | Y | Y | Y |
| List sessions | X | Y | Y | Y | Y |
| Get a single session | X | Y | Y | Y | Y |
| Like, dislike, or submit feedback | X | Y | Y | Y | Y |
| Batch delete sessions | X | X | Y | Y | Y |
| Update session name or information | X | X | Y | Y | Y |
| Delete a message and its reply or reference | X | X | Y | Y | Y |

For read permission, session operations are limited to the user's own sessions. Write permission can cover all sessions. Batch deletion and message deletion with write permission are limited to the user's own sessions; manage permission and owner permission cover all sessions.

### Chat generation and retrieval

| Operation | No permission | Read | Write | Manage | Owner |
| --- | --- | --- | --- | --- | --- |
| TTS returns audio stream | X | Y | Y | Y | Y |
| Audio to text | X | Y | Y | Y | Y |
| Main chat completion API, including SSE and non-SSE | X | Y | Y | Y | Y |
| OpenAI-compatible chat completion | X | Y | Y | Y | Y |
| Shared chatbot conversation entry | X | Y | Y | Y | Y |
| Get recommended questions | X | Y | Y | Y | Y |
| Legacy completion API | X | Y | Y | Y | Y |
| Legacy recommended questions | X | Y | Y | Y | Y |

For the main chat completion API, read permission applies to the user's own sessions, while manage permission applies to all sessions.
