# Change Logs - MonkeyOCR as DeepDoc Alternative

## Version 1.0.0 - Initial Implementation

### Backend API Changes

- **File**: `api/utils/validation_utils.py`

  - Added `ParserEngineEnum` with `deepdoc` and `monkeyocr` options
  - Added `parser_engine` field to `CreateDatasetReq` with default `deepdoc`
  - Updated `ChunkMethodnEnum` to include `monkeyocr`

- **File**: `api/apps/sdk/dataset.py`
  - Added `parser_engine` parameter handling in dataset creation
  - Maps `parser_engine` to `parser_config` and `parser_id` fields
  - Sets `parser_id = "monkeyocr"` when `parser_engine = "monkeyocr"`

### Processing Engine Integration

- **File**: `rag/svr/task_executor.py`

  - Added MonkeyOCR parser to `FACTORY` dictionary
  - Maps `ParserType.MONKEYOCR.value` to `monkey_ocr` chunk function

- **File**: `rag/app/monkey_ocr_parser.py`

  - Created core MonkeyOCR parser implementation
  - Follows exact `cedd_parse.py` flow with `full`, `parse_only`, `ocr_only` modes
  - Integrates with RAGFlow chunking system

- **File**: `rag/app/monkey_ocr_parser_mock.py`
  - Created mock version for testing without heavy model loading
  - Simulates all processing steps with print statements
  - Reads file content directly instead of OCR processing

### SDK Integration

- **File**: `sdk/python/ragflow_sdk/ragflow.py`
  - Added `parser_engine` parameter to `create_dataset` method
  - Updated docstring to document new parameter
  - Maintains backward compatibility with default `deepdoc`

### Documentation & Examples

- **File**: `examples/monkeyocr_deepdoc_alternative_example.py`
  - Created comprehensive usage example
  - Demonstrates both DeepDoc and MonkeyOCR usage
  - Shows API and SDK integration patterns

### Architecture Decisions

- **Database**: Used existing `parser_id` field with new values (Option B)
- **API**: Added `parser_engine` parameter to `CreateDatasetReq` (Option A)
- **SDK**: Kept exactly like DeepDoc dataset, only difference is the name
- **Configuration**: Used existing `parser_config` field (Option A)
- **Implementation Priority**: Backend API → SDK → Frontend → Processing Engine

### Testing Strategy

- Created mock parser for API testing without resource-intensive operations
- Mock version prints all processing steps and reads file content directly
- Easy switch between mock and real implementation via import comments

### Impact

- **New Feature**: Users can now choose between DeepDoc and MonkeyOCR as processing engines
- **Backward Compatibility**: All existing functionality remains unchanged
- **Extensibility**: Framework supports adding more processing engines in the future
