package task

// PrepareDataflowChunkAssets applies the minimal pre-index cleanup needed by
// the Go dataflow path so real pipeline output can be stored safely.
func PrepareDataflowChunkAssets(chunks []map[string]any) error {
	for _, ck := range chunks {
		delete(ck, "_pdf_positions")
		// FIXME: production parity with Python _prepare_docs_and_upload should
		// upload raw image data and set img_id. During the current bring-up phase
		// we drop raw image payloads so the main dataflow -> ES path can proceed.
		delete(ck, "image")
	}
	return nil
}
