# DeepSeek-OCR2 Setup Guide

DeepSeek-OCR2 is an advanced document OCR model using Visual Causal Flow technology.

## Installation

### Install Dependencies

```bash
pip install -r requirements-deepseek-ocr2.txt
```

Or install with pip extras:

```bash
pip install .[deepseek-ocr2]
```

### Flash Attention (Optional but Recommended)

For better performance with CUDA:

```bash
pip install flash-attn==2.7.3 --no-build-isolation
```

## Configuration

### Local Mode

Set in your configuration:

- **Model**: `deepseek-ai/DeepSeek-OCR-2`
- **Backend**: `local`

The model will be downloaded from HuggingFace on first use.

### HTTP API Mode

If using a remote DeepSeek-OCR2 API:

- **Backend**: `http`
- **API URL**: Your API endpoint
- **API Key**: Your API key (if required)

## Usage

Once configured, DeepSeek-OCR2 can be selected as the OCR model for document parsing.

## Features

- **Visual Causal Flow**: Simulates human "jumping reading" for better logical understanding
- **DeepEncoder V2**: Uses LLM as vision encoder for reasoning
- **Token Efficient**: 256-1120 tokens vs 6000+ for other models
- **Dual-stream Attention**: Bidirectional for global view + causal for reading order

## Troubleshooting

### CUDA Out of Memory

Try reducing image resolution or using the HTTP backend.

### Model Download Issues

Ensure you have internet access and sufficient disk space (~3GB).
