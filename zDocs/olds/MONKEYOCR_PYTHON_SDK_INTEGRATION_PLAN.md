# MonkeyOCR Integration with RAGFlow Python SDK

## 📋 Overview

This document outlines the plan to integrate MonkeyOCR as a **chunk method** in the RAGFlow Python SDK, allowing users to create datasets with advanced OCR capabilities through the `create_dataset()` method.

## 🎯 Current Status

### ✅ What's Already Implemented
- ✅ MonkeyOCR parser implementation (`rag/app/monkey_ocr_parser.py`)
- ✅ MonkeyOCR service layer (`api/db/services/monkeyocr_service.py`)
- ✅ REST API endpoints (`api/apps/monkeyocr_app.py`)
- ✅ Parser type registration (`ParserType.MONKEYOCR = "monkeyocr"`)
- ✅ Factory integration in document services
- ✅ Docker configuration and model support

### ❌ What's Missing
- ❌ MonkeyOCR not available as a **chunk method** in Python SDK
- ❌ Not listed in `ChunkMethodnEnum` validation
- ❌ No parser configuration for MonkeyOCR chunk method
- ❌ No SDK documentation for MonkeyOCR usage

## 🚀 Integration Plan

### Phase 1: Add MonkeyOCR as Chunk Method

#### 1.1 Update Validation Enum
**File**: `api/utils/validation_utils.py`

```python
class ChunkMethodnEnum(StrEnum):
    naive = auto()
    book = auto()
    email = auto()
    laws = auto()
    manual = auto()
    monkeyocr = auto()  # 🆕 ADD THIS
    one = auto()
    paper = auto()
    picture = auto()
    presentation = auto()
    qa = auto()
    table = auto()
    tag = auto()
```

#### 1.2 Define MonkeyOCR Parser Configuration
**File**: `api/utils/api_utils.py`

```python
def get_parser_config(chunk_method, parser_config):
    """Get parser configuration for chunk method"""

    # Add MonkeyOCR configuration
    if chunk_method == "monkeyocr":
        return {
            "split_pages": False,
            "pred_abandon": False,
            "extract_images": True,
            "generate_layout_pdf": False,
            "generate_spans_pdf": False,
            "enable_omr": True,
            "raptor": {"use_raptor": False}
        }

    # ... existing configurations ...
```

#### 1.3 Update Documentation Reference
**File**: Python SDK Documentation

Add to available chunk methods:
- `"monkeyocr"`: Advanced OCR and document layout analysis

### Phase 2: Configuration Schema

#### 2.1 MonkeyOCR Parser Config Schema
```python
class MonkeyOCRConfig(Base):
    split_pages: bool = Field(default=False, description="Split document by pages")
    pred_abandon: bool = Field(default=False, description="Predict abandoned elements")
    extract_images: bool = Field(default=True, description="Extract images from document")
    generate_layout_pdf: bool = Field(default=False, description="Generate layout visualization PDF")
    generate_spans_pdf: bool = Field(default=False, description="Generate spans visualization PDF")
    enable_omr: bool = Field(default=True, description="Enable Optical Mark Recognition")
    language: str = Field(default="Chinese", description="Document language")
    confidence_threshold: float = Field(default=0.8, ge=0.0, le=1.0, description="OCR confidence threshold")
```

#### 2.2 Default Configuration Mapping
```python
# chunk_method = "monkeyocr":
{
    "split_pages": False,
    "pred_abandon": False,
    "extract_images": True,
    "generate_layout_pdf": False,
    "generate_spans_pdf": False,
    "enable_omr": True,
    "language": "Chinese",
    "confidence_threshold": 0.8,
    "raptor": {"use_raptor": False}
}
```

### Phase 3: SDK Usage Examples

#### 3.1 Basic MonkeyOCR Dataset Creation
```python
from ragflow_sdk import RAGFlow

# Initialize RAGFlow client
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")

# Create dataset with MonkeyOCR chunk method
dataset = rag_object.create_dataset(
    name="ocr_documents",
    description="Dataset for scanned documents and images",
    chunk_method="monkeyocr",  # 🆕 NEW CHUNK METHOD
    embedding_model="BAAI/bge-large-zh-v1.5@BAAI"
)

print(f"Created dataset: {dataset.name}")
```

#### 3.2 Advanced MonkeyOCR Configuration
```python
from ragflow_sdk import RAGFlow, DataSet

# Custom MonkeyOCR configuration
monkeyocr_config = DataSet.ParserConfig(
    split_pages=True,          # Split by pages for large documents
    pred_abandon=True,         # Skip uncertain elements
    extract_images=True,       # Extract embedded images
    generate_layout_pdf=True,  # Generate layout visualization
    enable_omr=True,          # Enable form recognition
    language="English",        # Set language
    confidence_threshold=0.9   # High confidence threshold
)

# Create dataset with custom config
dataset = rag_object.create_dataset(
    name="advanced_ocr_docs",
    description="Advanced OCR processing with layout analysis",
    chunk_method="monkeyocr",
    parser_config=monkeyocr_config
)
```

#### 3.3 MonkeyOCR for Specific Use Cases
```python
# For scanned PDFs and documents
scanned_docs_dataset = rag_object.create_dataset(
    name="scanned_documents",
    chunk_method="monkeyocr",
    parser_config=DataSet.ParserConfig(
        split_pages=False,
        extract_images=True,
        language="Chinese"
    )
)

# For forms and surveys (OMR)
forms_dataset = rag_object.create_dataset(
    name="survey_forms",
    chunk_method="monkeyocr",
    parser_config=DataSet.ParserConfig(
        enable_omr=True,
        pred_abandon=True,
        confidence_threshold=0.9
    )
)

# For academic papers with formulas
academic_dataset = rag_object.create_dataset(
    name="research_papers",
    chunk_method="monkeyocr",
    parser_config=DataSet.ParserConfig(
        extract_images=True,
        generate_layout_pdf=True,
        language="English"
    )
)
```

### Phase 4: Implementation Tasks

#### 4.1 Code Changes Required

1. **Update Validation Enum** ⭐ HIGH PRIORITY
   - Add `monkeyocr = auto()` to `ChunkMethodnEnum`
   - File: `api/utils/validation_utils.py`

2. **Add Parser Configuration** ⭐ HIGH PRIORITY
   - Update `get_parser_config()` function
   - File: `api/utils/api_utils.py`

3. **Update Documentation** ⭐ MEDIUM PRIORITY
   - Add MonkeyOCR to SDK documentation
   - Update parser configuration examples

4. **Add Configuration Schema** ⭐ MEDIUM PRIORITY
   - Create `MonkeyOCRConfig` class
   - Add validation for MonkeyOCR-specific parameters

#### 4.2 Testing Requirements

1. **Unit Tests**
   ```python
   def test_monkeyocr_chunk_method():
       """Test MonkeyOCR chunk method creation"""
       dataset = rag_object.create_dataset(
           name="test_monkeyocr",
           chunk_method="monkeyocr"
       )
       assert dataset is not None
       assert dataset.chunk_method == "monkeyocr"
   ```

2. **Integration Tests**
   ```python
   def test_monkeyocr_document_processing():
       """Test document upload and processing with MonkeyOCR"""
       # Upload scanned PDF
       # Verify OCR processing
       # Check chunk generation
   ```

3. **Configuration Tests**
   ```python
   def test_monkeyocr_custom_config():
       """Test custom MonkeyOCR configuration"""
       # Test various config combinations
       # Verify parameter validation
   ```

### Phase 5: Deployment Strategy

#### 5.1 Rollout Plan

1. **Development Phase** (Week 1-2)
   - Implement core enum and configuration changes
   - Add basic MonkeyOCR chunk method support
   - Create unit tests

2. **Testing Phase** (Week 3)
   - Integration testing with various document types
   - Performance testing with large documents
   - Configuration validation testing

3. **Documentation Phase** (Week 4)
   - Update Python SDK documentation
   - Create usage examples and tutorials
   - Update API reference

4. **Production Release** (Week 5)
   - Deploy to production environment
   - Monitor usage and performance
   - Gather user feedback

#### 5.2 Compatibility Considerations

- **Backward Compatibility**: All existing chunk methods remain unchanged
- **Optional Feature**: MonkeyOCR requires additional model weights
- **Performance Impact**: Consider memory usage for MonkeyOCR model loading
- **Error Handling**: Graceful fallback if MonkeyOCR model unavailable

### Phase 6: User Documentation

#### 6.1 SDK Documentation Update

```markdown
##### chunk_method: `str`

The chunking method of the dataset to create. Available options:

* `"naive"`: General (default)
* `"manual"`: Manual
* `"qa"`: Q&A
* `"table"`: Table
* `"paper"`: Paper
* `"book"`: Book
* `"laws"`: Laws
* `"presentation"`: Presentation
* `"picture"`: Picture
* `"one"`: One
* `"email"`: Email
* `"monkeyocr"`: Advanced OCR with layout analysis 🆕
```

#### 6.2 Configuration Documentation

```markdown
* `chunk_method`=`"monkeyocr"`:
  ```json
  {
    "split_pages": false,
    "pred_abandon": false,
    "extract_images": true,
    "generate_layout_pdf": false,
    "generate_spans_pdf": false,
    "enable_omr": true,
    "language": "Chinese",
    "confidence_threshold": 0.8,
    "raptor": {"use_raptor": false}
  }
  ```
```

## 🎯 Success Criteria

### Functional Requirements
- ✅ Users can create datasets with `chunk_method="monkeyocr"`
- ✅ MonkeyOCR configuration parameters are validated
- ✅ Documents are processed using MonkeyOCR parser
- ✅ OCR results are properly chunked and stored

### Performance Requirements
- ✅ MonkeyOCR processing completes within reasonable time
- ✅ Memory usage remains manageable
- ✅ Error handling for unsupported files

### Documentation Requirements
- ✅ Updated Python SDK documentation
- ✅ Usage examples and tutorials
- ✅ Configuration parameter reference

## 🔧 Development Environment Setup

### Prerequisites
1. RAGFlow development environment
2. MonkeyOCR model weights downloaded
3. CUDA support (optional but recommended)

### Setup Commands
```bash
# Clone RAGFlow repository
git clone https://github.com/infiniflow/ragflow.git
cd ragflow

# Install MonkeyOCR dependencies
cd monkeyocr
pip install -r requirements.txt

# Download MonkeyOCR models
python tools/download_model.py -t quantize

# Run tests
python -m pytest tests/test_monkeyocr_integration.py
```

## 📊 Timeline and Milestones

| Phase | Duration | Deliverables |
|-------|----------|--------------|
| Phase 1 | 1-2 weeks | Core enum and config changes |
| Phase 2 | 1 week | Configuration schema |
| Phase 3 | 1 week | SDK usage examples |
| Phase 4 | 2 weeks | Implementation and testing |
| Phase 5 | 1 week | Deployment and monitoring |
| Phase 6 | 1 week | Documentation updates |

**Total Timeline**: 6-8 weeks

## 🚨 Risk Assessment

### Technical Risks
- **Model Loading**: MonkeyOCR model loading time may affect performance
- **Memory Usage**: Large documents may consume significant memory
- **GPU Dependencies**: CUDA requirements may limit deployment options

### Mitigation Strategies
- **Lazy Loading**: Load MonkeyOCR model only when needed
- **Memory Management**: Implement proper cleanup and garbage collection
- **Fallback Options**: Provide CPU-only processing option

## 📈 Future Enhancements

### Short-term (Next 3 months)
- Performance optimizations
- Additional language support
- Batch processing capabilities

### Long-term (6+ months)
- Custom model support
- Real-time OCR processing
- Integration with RAGFlow's graph-based chunking

## 📞 Contact and Support

For questions about this integration plan, contact:
- Development Team: [team@ragflow.io](mailto:team@ragflow.io)
- Documentation: [docs@ragflow.io](mailto:docs@ragflow.io)
- Technical Support: [support@ragflow.io](mailto:support@ragflow.io)

---

**Created**: December 2024
**Last Updated**: December 2024
**Version**: 1.0
**Status**: Planning Phase
