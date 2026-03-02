// 会话历史类型定义

export interface ConversationHistory {
  id: string; // conversation_id
  dialog_id: string;
  dialog_name: string;
  dialog_icon?: string;
  title: string;
  last_message: string;
  message_count: number;
  create_time: number; // Unix timestamp
  update_time: number; // Unix timestamp
}
