export interface IFeedbackRequestBody {
  messageId?: string;
  thumbup?: boolean;
  feedback?: string;
}

export interface IAskRequestBody {
  question: string;
  kb_ids: string[];
}
