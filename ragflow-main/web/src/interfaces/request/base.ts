export interface IPaginationRequestBody {
  keywords?: string;
  page?: number;
  page_size?: number; // name|create|doc_num|create_time|update_time, default：create_time
  orderby?: string;
  desc?: string;
}
