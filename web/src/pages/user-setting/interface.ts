export interface ApiKeyPostBody {
  api_key: string;
  base_url: string;
  group_id?: string;
  // WhisperX specific configuration
  enable_diarization?: boolean;
  min_speakers?: number;
  max_speakers?: number;
  initial_prompt?: string;
  condition_on_previous_text?: boolean;
  diarization_batch_size?: number;

  // WhisperX ASR Options
  beam_size?: number;
  best_of?: number;
  vad_onset?: number;
  vad_offset?: number;
}
