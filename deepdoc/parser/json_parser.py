# -*- coding: utf-8 -*-
# The following documents are mainly referenced, and only adaptation modifications have been made
# from https://github.com/langchain-ai/langchain/blob/master/libs/text-splitters/langchain_text_splitters/json.py

import json
from typing import Any, Dict, List, Optional
from rag.nlp import find_codec

class RAGFlowJsonParser:
    def __init__(
        self, max_chunk_size: int = 2000, min_chunk_size: Optional[int] = None
    ):
        super().__init__()
        self.max_chunk_size = max_chunk_size * 2
        self.min_chunk_size = (
            min_chunk_size
            if min_chunk_size is not None
            else max(max_chunk_size - 200, 50)
        )

    def __call__(self, binary):
        encoding = find_codec(binary)
        txt = binary.decode(encoding, errors="ignore")
        json_data = json.loads(txt)
        chunks = self.split_json(json_data, True)   
        sections = [json.dumps(l, ensure_ascii=False) for l in chunks if l]
        return sections

    @staticmethod
    def _json_size(data: Dict) -> int:
        """Calculate the size of the serialized JSON object."""
        return len(json.dumps(data, ensure_ascii=False))

    @staticmethod
    def _set_nested_dict(d: Dict, path: List[str], value: Any) -> None:
        """Set a value in a nested dictionary based on the given path."""
        for key in path[:-1]:
            d = d.setdefault(key, {})
        d[path[-1]] = value

    def _list_to_dict_preprocessing(self, data: Any) -> Any:
        if isinstance(data, dict):
            # Process each key-value pair in the dictionary
            return {k: self._list_to_dict_preprocessing(v) for k, v in data.items()}
        elif isinstance(data, list):
            # Convert the list to a dictionary with index-based keys
            return {
                str(i): self._list_to_dict_preprocessing(item)
                for i, item in enumerate(data)
            }
        else:
            # Base case: the item is neither a dict nor a list, so return it unchanged
            return data
        
    def _json_split(
        self,
        data: Dict[str, Any],
        current_path: Optional[List[str]] = None,
        chunks: Optional[List[Dict]] = None,
    ) -> List[Dict]:
        """
        Split json into maximum size dictionaries while preserving structure.
        """
        current_path = current_path or []
        chunks = chunks or [{}]
        if isinstance(data, dict):
            for key, value in data.items():
                new_path = current_path + [key]
                chunk_size = self._json_size(chunks[-1])
                size = self._json_size({key: value})
                remaining = self.max_chunk_size - chunk_size

                if size < remaining:
                    # Add item to current chunk
                    self._set_nested_dict(chunks[-1], new_path, value)
                else:
                    if chunk_size >= self.min_chunk_size:
                        # Chunk is big enough, start a new chunk
                        chunks.append({})

                    # Iterate
                    self._json_split(value, new_path, chunks)
        else:
            # handle single item
            self._set_nested_dict(chunks[-1], current_path, data)
        return chunks

    def split_json(
        self,
        json_data: Dict[str, Any],
        convert_lists: bool = False,
    ) -> List[Dict]:
        """Splits JSON into a list of JSON chunks"""

        if convert_lists:
            chunks = self._json_split(self._list_to_dict_preprocessing(json_data))
        else:
            chunks = self._json_split(json_data)

        # Remove the last chunk if it's empty
        if not chunks[-1]:
            chunks.pop()
        return chunks

    def split_text(
        self,
        json_data: Dict[str, Any],
        convert_lists: bool = False,
        ensure_ascii: bool = True,
    ) -> List[str]:
        """Splits JSON into a list of JSON formatted strings"""

        chunks = self.split_json(json_data=json_data, convert_lists=convert_lists)

        # Convert to string
        return [json.dumps(chunk, ensure_ascii=ensure_ascii) for chunk in chunks]
