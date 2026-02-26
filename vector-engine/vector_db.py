"""ChromaDB 向量数据库封装"""

import os
import time
import chromadb
from embedding import EmbeddingModel, ChromaEmbeddingFunction


class VectorDB:
    # Shared singleton — avoids loading the model twice (saves ~25s on Windows)
    _shared_model: EmbeddingModel | None = None
    _shared_fn: ChromaEmbeddingFunction | None = None

    @classmethod
    def _get_shared_model(cls) -> tuple[EmbeddingModel, ChromaEmbeddingFunction]:
        if cls._shared_model is None:
            cls._shared_model = EmbeddingModel()
            cls._shared_fn = ChromaEmbeddingFunction(cls._shared_model)
        return cls._shared_model, cls._shared_fn

    def __init__(self, db_path: str = None, collection_name: str = "files"):
        self.db_path = db_path or os.getenv(
            "VECTOR_DB_PATH",
            os.path.expanduser("~/.ai-hub/vector-engine/data"),
        )
        os.makedirs(self.db_path, exist_ok=True)
        self.embedding_model, self.embedding_fn = self._get_shared_model()
        self.client = chromadb.PersistentClient(path=self.db_path)
        self.collection = self.client.get_or_create_collection(
            name=collection_name, embedding_function=self.embedding_fn
        )

    def add(self, doc_id: str, text: str, metadata: dict = None):
        """添加或更新一条向量记录"""
        now = time.strftime("%Y-%m-%dT%H:%M:%S")
        meta = metadata or {}
        meta.setdefault("created_at", now)
        meta["updated_at"] = now
        meta.setdefault("hit_count", 0)
        meta.setdefault("last_hit_time", "")
        # upsert: 存在则更新，不存在则插入
        self.collection.upsert(ids=[doc_id], documents=[text], metadatas=[meta])

    def search(self, query: str, top_k: int = 5) -> list[dict]:
        """语义搜索，返回相似度排序结果"""
        count = self.collection.count()
        if count == 0:
            return []
        actual_k = min(top_k, count)
        results = self.collection.query(query_texts=[query], n_results=actual_k)
        items = []
        for i in range(len(results["ids"][0])):
            distance = results["distances"][0][i] if results["distances"] else 0
            similarity = max(0, min(1, 1 - distance / 2))
            meta = results["metadatas"][0][i] if results["metadatas"] else {}
            items.append({
                "id": results["ids"][0][i],
                "document": results["documents"][0][i] if results["documents"] else "",
                "similarity": round(similarity, 4),
                "metadata": meta,
            })
        return items

    def delete(self, doc_id: str):
        """删除一条向量记录"""
        try:
            self.collection.delete(ids=[doc_id])
        except Exception:
            pass

    def get(self, doc_id: str) -> dict | None:
        """获取单条记录"""
        result = self.collection.get(ids=[doc_id])
        if not result["ids"]:
            return None
        return {
            "id": result["ids"][0],
            "document": result["documents"][0] if result["documents"] else "",
            "metadata": result["metadatas"][0] if result["metadatas"] else {},
        }

    def record_hit(self, doc_id: str):
        """记录一次命中"""
        record = self.get(doc_id)
        if not record:
            return
        meta = record["metadata"]
        meta["hit_count"] = meta.get("hit_count", 0) + 1
        meta["last_hit_time"] = time.strftime("%Y-%m-%dT%H:%M:%S")
        self.collection.update(ids=[doc_id], metadatas=[meta])

    def stats(self) -> dict:
        """返回统计信息"""
        count = self.collection.count()
        if count == 0:
            return {"total": 0, "records": []}
        all_data = self.collection.get()
        records = []
        for i in range(len(all_data["ids"])):
            meta = all_data["metadatas"][i] if all_data["metadatas"] else {}
            records.append({
                "id": all_data["ids"][i],
                "hit_count": meta.get("hit_count", 0),
                "last_hit_time": meta.get("last_hit_time", ""),
            })
        records.sort(key=lambda x: x["hit_count"], reverse=True)
        return {"total": count, "records": records}
