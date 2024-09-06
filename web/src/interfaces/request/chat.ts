export interface IFeedbackRequestBody {
  messageId?: string;
  thumbup?: boolean;
  feedback?: string;
}

export interface IAskRequestBody {
  questionkb_ids: string;
  kb_ids: string[];
}
