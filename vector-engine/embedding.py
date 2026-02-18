"""Embedding 模型封装 - BAAI/bge-small-zh-v1.5"""

import os
from sentence_transformers import SentenceTransformer
import chromadb.api.types as types


class EmbeddingModel:
    def __init__(self, model_name: str = None, cache_dir: str = None):
        self.model_name = model_name or os.getenv(
            "EMBEDDING_MODEL_NAME", "BAAI/bge-small-zh-v1.5"
        )
        self.cache_dir = cache_dir or os.getenv(
            "EMBEDDING_MODEL_PATH",
            os.path.expanduser("~/.ai-hub/vector-engine/models"),
        )
        os.makedirs(self.cache_dir, exist_ok=True)
        self.model = SentenceTransformer(
            self.model_name, device="cpu", cache_folder=self.cache_dir
        )

    def encode(self, texts: list[str]) -> list[list[float]]:
        embeddings = self.model.encode(texts, show_progress_bar=False)
        return embeddings.tolist()


class ChromaEmbeddingFunction(types.EmbeddingFunction):
    """适配 ChromaDB 的 embedding 函数"""

    def __init__(self, model: EmbeddingModel):
        self.model = model

    def __call__(self, input: list[str]) -> list[list[float]]:
        return self.model.encode(input)
