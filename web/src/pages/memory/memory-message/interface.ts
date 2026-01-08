export interface IMessageInfo {
  message_id: number;
  message_type: 'semantic' | 'raw' | 'procedural';
  source_id: string | '-';
  user_id: string;
  agent_id: string;
  agent_name: string;
  session_id: string;
  valid_at: string;
  invalid_at: string;
  forget_at: string;
  status: boolean;
  extract?: IMessageInfo[];
}

export interface IMessageTableProps {
  messages: { message_list: Array<IMessageInfo>; total_count: number };
  storage_type: string;
}

export interface IMessageContentProps {
  content: string;
  content_embed: string;
}
