# MonkeyOCR Integration - Phase 1 Implementation

## Overview

This document describes the Phase 1 implementation of MonkeyOCR integration with RAGFlow. The integration provides advanced document parsing, OCR, and OMR (Optical Mark Recognition) capabilities to the RAGFlow platform.

## Architecture

### Core Components

1. **MonkeyOCR Processor** (`deepdoc/vision/monkey_ocr.py`)
   - Core integration layer that wraps MonkeyOCR functionality
   - Provides clean API for document parsing and OCR
   - Handles model initialization and management

2. **RAG Parser** (`rag/app/monkey_ocr_parser.py`)
   - RAGFlow-specific parser that integrates with existing parser system
   - Provides factory pattern for parser creation
   - Handles RAGFlow-compatible output formats

3. **Configuration** (`conf/monkey_ocr_config.json`)
   - Comprehensive configuration for MonkeyOCR integration
   - Settings for performance, capabilities, and output options
   - Integration settings for RAGFlow

## Features

### Document Parsing
- **PDF Processing**: Full PDF document analysis with layout detection
- **Image Processing**: Support for JPG, JPEG, PNG images
- **Layout Analysis**: Automatic detection of document structure
- **Content Extraction**: Text, formulas, and table extraction

### OCR Capabilities
- **Text Recognition**: High-accuracy text extraction from images
- **Formula Recognition**: LaTeX format formula extraction
- **Table Recognition**: HTML format table extraction
- **Batch Processing**: Efficient batch processing of multiple images

### OMR (Optical Mark Recognition)
- **Circle Detection**: Automatic detection of multiple choice forms
- **Rating Scale Processing**: 5-point rating scale extraction
- **Pattern Recognition**: Regular pattern detection for forms
- **Score Calculation**: Automatic scoring from marked circles

### Performance Features
- **GPU Acceleration**: CUDA support for faster processing
- **Batch Processing**: Efficient batch inference
- **Memory Management**: Optimized memory usage
- **Caching**: Configurable caching for improved performance

## File Structure

```
ragflow/
├── deepdoc/
│   └── vision/
│       └── monkey_ocr.py              # Core MonkeyOCR processor
├── rag/
│   └── app/
│       └── monkey_ocr_parser.py       # RAGFlow parser integration
├── conf/
│   └── monkey_ocr_config.json         # Configuration file
├── monkeyocr/                         # MonkeyOCR source code
│   ├── cedd_parse.py                  # Main MonkeyOCR script
│   ├── model_configs.yaml             # MonkeyOCR model config
│   ├── requirements.txt               # MonkeyOCR dependencies
│   └── ...
└── test_monkeyocr_integration.py      # Integration test script
```

## Installation

### Prerequisites

1. **MonkeyOCR Dependencies**
   ```bash
   cd monkeyocr
   pip install -r requirements.txt
   ```

2. **Model Weights**
   ```bash
   cd monkeyocr/tools
   python download_model.py --type huggingface --name MonkeyOCR
   ```

3. **System Requirements**
   - Python 3.8+
   - CUDA-compatible GPU (recommended)
   - OpenCV
   - PDF2Image
   - PIL/Pillow

### Configuration

1. **Update MonkeyOCR Config**
   Edit `monkeyocr/model_configs.yaml` to match your system:
   ```yaml
   device: cuda  # or cpu
   models_dir: model_weight
   ```

2. **RAGFlow Integration Config**
   The integration configuration is in `conf/monkey_ocr_config.json` and includes:
   - Parser settings
   - Performance options
   - Output preferences
   - OMR parameters

## Usage

### Basic Document Parsing

```python
from rag.app.monkey_ocr_parser import MonkeyOCRParser

# Initialize parser
parser = MonkeyOCRParser()

# Parse document
result = parser.parse_document(
    file_path="document.pdf",
    output_dir="./output",
    split_pages=False
)

if result['success']:
    print(f"Content: {result['content']}")
    print(f"Metadata: {result['metadata']}")
```

### Image OCR

```python
from rag.app.monkey_ocr_parser import MonkeyOCRParser

parser = MonkeyOCRParser()

# Extract text from images
image_paths = ["image1.jpg", "image2.png"]
text_results = parser.extract_text_from_images(
    image_paths=image_paths,
    task="text"  # or "formula", "table"
)

for filename, text in text_results.items():
    print(f"{filename}: {text}")
```

### Advanced Features

```python
from deepdoc.vision.monkey_ocr import MonkeyOCRProcessor

# Direct processor access
processor = MonkeyOCRProcessor()

# Parse with custom options
parsed_dir = processor.parse_document(
    input_file="document.pdf",
    output_dir="./output",
    split_pages=True,
    pred_abandon=True
)

# Get structured content
content_data = processor.get_parsed_content(parsed_dir)
print(f"Markdown: {content_data['markdown_content']}")
print(f"Content List: {content_data['content_list']}")
```

## Configuration Options

### Parser Settings
- `split_pages`: Split results by pages
- `pred_abandon`: Predict abandon elements
- `extract_images`: Extract images from documents
- `generate_layout_pdf`: Generate layout visualization
- `generate_spans_pdf`: Generate spans visualization

### Performance Settings
- `enable_caching`: Enable result caching
- `cache_dir`: Cache directory path
- `max_cache_size`: Maximum cache size
- `enable_parallel_processing`: Enable parallel processing
- `max_workers`: Maximum worker threads

### OMR Settings
- `mean_percent_threshold`: Threshold for circle detection
- `min_pixel_count`: Minimum pixel count for circles
- `circle_aspect_ratio_min/max`: Circle aspect ratio bounds
- `circle_size_min/max`: Circle size bounds

## Testing

Run the integration test script:

```bash
python test_monkeyocr_integration.py
```

This will test:
- MonkeyOCR installation
- Configuration loading
- Processor initialization
- Parser functionality

## Integration with RAGFlow

### Parser Registry
The MonkeyOCR parser can be registered with RAGFlow's parser system:

```python
from rag.app.monkey_ocr_parser import MonkeyOCRFactory

# Get parser info
info = MonkeyOCRFactory.get_parser_info()
print(f"Parser: {info['name']} v{info['version']}")

# Create parser instance
parser = MonkeyOCRFactory.create_parser()
```

### Supported Formats
- PDF documents
- JPG/JPEG images
- PNG images

### Output Formats
- Markdown content
- JSON content lists
- Layout visualizations
- Span visualizations

## Performance Considerations

### Memory Usage
- Large documents may require significant memory
- Consider using `split_pages=True` for large PDFs
- Monitor GPU memory usage with CUDA

### Processing Speed
- GPU acceleration significantly improves speed
- Batch processing is more efficient than single images
- Caching can improve repeated processing

### Optimization Tips
1. Use appropriate batch sizes for your hardware
2. Enable caching for repeated documents
3. Use parallel processing for multiple documents
4. Monitor memory usage and adjust accordingly

## Troubleshooting

### Common Issues

1. **Model Loading Failures**
   - Ensure model weights are downloaded
   - Check CUDA installation for GPU mode
   - Verify model_configs.yaml path

2. **Import Errors**
   - Check Python path includes monkeyocr directory
   - Verify all dependencies are installed
   - Check for version conflicts

3. **Memory Issues**
   - Reduce batch size
   - Use CPU mode if GPU memory is insufficient
   - Enable page splitting for large documents

### Debug Mode
Enable debug logging in configuration:
```json
{
  "monkeyocr": {
    "logging": {
      "level": "DEBUG"
    }
  }
}
```

## Next Steps (Phase 2)

Phase 2 will include:
- Database integration for parser registration
- API endpoints for MonkeyOCR functionality
- Web interface integration
- Advanced caching and optimization
- Extended OMR capabilities

## Support

For issues and questions:
1. Check the troubleshooting section
2. Review MonkeyOCR documentation
3. Check RAGFlow integration logs
4. Verify configuration settings

## License

This integration follows the same license as the main RAGFlow project and MonkeyOCR. 